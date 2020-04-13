package index

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/treeverse/lakefs/logging"

	"github.com/treeverse/lakefs/index/dag"

	"github.com/treeverse/lakefs/index/errors"

	"github.com/treeverse/lakefs/db"
	"github.com/treeverse/lakefs/ident"
	"github.com/treeverse/lakefs/index/merkle"
	"github.com/treeverse/lakefs/index/model"
	pth "github.com/treeverse/lakefs/index/path"
	"github.com/treeverse/lakefs/index/store"

	"golang.org/x/xerrors"
)

const (
	// DefaultPartialCommitRatio is the ratio (1/?) of writes that will trigger a partial commit (number between 0-1)
	DefaultPartialCommitRatio = 1 // 1 writes before a partial commit

	// DefaultBranch is the branch to be automatically created when a repo is born
	DefaultBranch = "master"
)

type Index interface {
	WithContext(ctx context.Context) Index
	Tree(repoId, branch string) error
	ReadObject(repoId, ref, path string) (*model.Object, error)
	ReadEntryObject(repoId, ref, path string) (*model.Entry, error)
	ReadEntryTree(repoId, ref, path string) (*model.Entry, error)
	ReadRootObject(repoId, ref string) (*model.Root, error)
	WriteObject(repoId, branch, path string, object *model.Object) error
	WriteEntry(repoId, branch, path string, entry *model.Entry) error
	WriteFile(repoId, branch, path string, entry *model.Entry, obj *model.Object) error
	DeleteObject(repoId, branch, path string) error
	ListObjectsByPrefix(repoId, ref, path, after string, results int, descend bool) ([]*model.Entry, bool, error)
	ListBranchesByPrefix(repoId string, prefix string, amount int, after string) ([]*model.Branch, bool, error)
	ResetBranch(repoId, branch string) error
	CreateBranch(repoId, branch, ref string) (*model.Branch, error)
	GetBranch(repoId, branch string) (*model.Branch, error)
	Commit(repoId, branch, message, committer string, metadata map[string]string) (*model.Commit, error)
	GetCommit(repoId, commitId string) (*model.Commit, error)
	GetCommitLog(repoId, fromCommitId string, results int, after string) ([]*model.Commit, bool, error)
	DeleteBranch(repoId, branch string) error
	Diff(repoId, leftRef, rightRef string) (merkle.Differences, error)
	DiffWorkspace(repoId, branch string) (merkle.Differences, error)
	RevertCommit(repoId, branch, commit string) error
	RevertPath(repoId, branch, path string) error
	RevertObject(repoId, branch, path string) error
	Merge(repoId, source, destination, userId string) (merkle.Differences, error)
	CreateRepo(repoId, bucketName, defaultBranch string) error
	ListRepos(amount int, after string) ([]*model.Repo, bool, error)
	GetRepo(repoId string) (*model.Repo, error)
	DeleteRepo(repoId string) error
}

func writeEntryToWorkspace(tx store.RepoOperations, repo *model.Repo, branch, path string, entry *model.WorkspaceEntry) error {
	err := tx.WriteToWorkspacePath(branch, path, entry)
	if err != nil {
		return err
	}
	if shouldPartiallyCommit(repo) {
		err = partialCommit(tx, branch)
		if err != nil {
			return err
		}
	}
	return nil
}

func shouldPartiallyCommit(repo *model.Repo) bool {
	chosen := rand.Float32()
	return chosen < repo.GetPartialCommitRatio()
}

func partialCommit(tx store.RepoOperations, branch string) error {
	// see if we have any changes that weren't applied
	wsEntries, err := tx.ListWorkspace(branch)
	if err != nil {
		return err
	}
	if len(wsEntries) == 0 {
		return nil
	}

	// get branch info (including current workspace root)
	branchData, err := tx.ReadBranch(branch)
	if xerrors.Is(err, db.ErrNotFound) {
		return nil
	} else if err != nil {
		return err // unexpected error
	}

	// update the immutable Merkle tree, getting back a new tree
	tree := merkle.New(branchData.GetWorkspaceRoot())
	tree, err = tree.Update(tx, wsEntries)
	if err != nil {
		return err
	}

	// clear workspace entries
	err = tx.ClearWorkspace(branch)
	if err != nil {
		return err
	}

	// update branch pointer to point at new workspace
	err = tx.WriteBranch(branch, &model.Branch{
		Name:          branch,
		Commit:        branchData.GetCommit(),
		CommitRoot:    branchData.GetCommitRoot(),
		WorkspaceRoot: tree.Root(), // does this happen properly?
	})
	if err != nil {
		return err
	}

	// done!
	return nil
}

func gc(tx store.RepoOperations, addr string) {
	// TODO: impl? here?
}

type KVIndex struct {
	kv          store.Store
	tsGenerator TimeGenerator

	ctx context.Context
}

type Option func(index *KVIndex)

type TimeGenerator func() int64

// Option to initiate with
// when using this option timestamps will generate using the given time generator
// used for mocking and testing timestamps
func WithTimeGenerator(generator TimeGenerator) Option {
	return func(kvi *KVIndex) {
		kvi.tsGenerator = generator
	}
}

func WithContext(ctx context.Context) Option {
	return func(kvi *KVIndex) {
		kvi.ctx = ctx
	}
}

func NewKVIndex(kv store.Store, opts ...Option) *KVIndex {
	kvi := &KVIndex{
		kv:          kv,
		tsGenerator: func() int64 { return time.Now().Unix() },
		ctx:         context.Background(),
	}
	for _, opt := range opts {
		opt(kvi)
	}
	return kvi
}

type reference struct {
	commit   *model.Commit
	branch   *model.Branch
	isBranch bool
}

func (r *reference) String() string {
	if r.isBranch {
		return fmt.Sprintf("[branch='%s' -> commit='%s' -> root='%s']",
			r.branch.GetName(),
			r.commit.GetAddress(),
			r.commit.GetTree())
	}
	return fmt.Sprintf("[commit='%s' -> root='%s']",
		r.commit.GetAddress(),
		r.commit.GetTree())
}

func resolveRef(tx store.RepoReadOnlyOperations, ref string) (*reference, error) {
	// if this is not
	if ident.IsHash(ref) {
		// this looks like a straight up commit, let's see if it exists
		commit, err := tx.ReadCommit(ref)
		if err != nil && !xerrors.Is(err, db.ErrNotFound) {
			// got an error, we can't continue
			return nil, err
		} else if err == nil {
			// great, it's a commit, return it
			return &reference{
				commit: commit,
			}, nil
		}
	}
	// treat this as a branch name
	branch, err := tx.ReadBranch(ref)
	if err != nil {
		return nil, err
	}
	commit, err := tx.ReadCommit(branch.GetCommit())
	if err != nil {
		return nil, err
	}

	return &reference{
		commit:   commit,
		branch:   branch,
		isBranch: true,
	}, nil
}

func (index *KVIndex) log() logging.Logger {
	return logging.FromContext(index.ctx).WithField("service_name", "index")
}

// Business logic
func (index *KVIndex) WithContext(ctx context.Context) Index {
	return &KVIndex{
		kv:          index.kv,
		tsGenerator: index.tsGenerator,
		ctx:         ctx,
	}
}

func (index *KVIndex) ReadObject(repoId, ref, path string) (*model.Object, error) {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(ref),
		ValidatePath(path))
	if err != nil {
		return nil, err
	}

	obj, err := index.kv.RepoReadTransact(repoId, func(tx store.RepoReadOnlyOperations) (interface{}, error) {
		_, err := tx.ReadRepo()
		if err != nil {
			return nil, err
		}

		reference, err := resolveRef(tx, ref)
		if err != nil {
			return nil, err
		}
		var obj *model.Object

		if reference.isBranch {
			we, err := tx.ReadFromWorkspace(reference.branch.GetName(), path)
			if xerrors.Is(err, db.ErrNotFound) {
				// not in workspace, let's try reading it from branch tree
				m := merkle.New(reference.branch.GetWorkspaceRoot())
				obj, err = m.GetObject(tx, path)
				if err != nil {
					return nil, err
				}
				return obj, nil
			} else if err != nil {
				// an actual error has occurred, return it.
				index.log().WithError(err).Error("could not read from workspace")
				return nil, err
			}
			if we.GetTombstone() {
				// object was deleted deleted
				return nil, db.ErrNotFound
			}
			return tx.ReadObject(we.GetEntry().GetAddress())
		}
		// otherwise, read from commit
		m := merkle.New(reference.commit.GetTree())
		obj, err = m.GetObject(tx, path)
		if err != nil {
			return nil, err
		}
		return obj, nil
	})
	if err != nil {
		return nil, err
	}
	return obj.(*model.Object), nil
}

func readEntry(tx store.RepoReadOnlyOperations, ref, path string, typ model.Entry_Type) (*model.Entry, error) {
	var entry *model.Entry

	_, err := tx.ReadRepo()
	if err != nil {
		return nil, err
	}

	reference, err := resolveRef(tx, ref)
	if err != nil {
		return nil, err
	}
	root := reference.commit.GetTree()
	if reference.isBranch {
		// try reading from workspace
		we, err := tx.ReadFromWorkspace(reference.branch.GetName(), path)

		// continue with we only if we got no error
		if err != nil {
			if !xerrors.Is(err, db.ErrNotFound) {
				return nil, err
			}
		} else {
			if we.GetTombstone() {
				// object was deleted deleted
				return nil, db.ErrNotFound
			}
			return we.GetEntry(), nil
		}
		root = reference.branch.GetWorkspaceRoot()
	}

	m := merkle.New(root)
	entry, err = m.GetEntry(tx, path, typ)
	if err != nil {
		return nil, err
	}
	return entry, nil
}

func (index *KVIndex) ReadEntry(repoId, branch, path string, typ model.Entry_Type) (*model.Entry, error) {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(branch),
		ValidatePath(path))
	if err != nil {
		return nil, err
	}
	entry, err := index.kv.RepoReadTransact(repoId, func(tx store.RepoReadOnlyOperations) (interface{}, error) {
		return readEntry(tx, branch, path, typ)
	})
	if err != nil {
		return nil, err
	}
	return entry.(*model.Entry), nil
}

func (index *KVIndex) ReadRootObject(repoId, ref string) (*model.Root, error) {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(ref))
	if err != nil {
		return nil, err
	}
	root, err := index.kv.RepoReadTransact(repoId, func(tx store.RepoReadOnlyOperations) (i interface{}, err error) {
		_, err = tx.ReadRepo()
		if err != nil {
			return nil, err
		}
		reference, err := resolveRef(tx, ref)
		if err != nil {
			return nil, err
		}
		if reference.isBranch {
			return tx.ReadRoot(reference.branch.GetWorkspaceRoot())
		}
		return tx.ReadRoot(reference.commit.GetTree())
	})
	if err != nil {
		return nil, err
	}
	return root.(*model.Root), nil
}

func (index *KVIndex) ReadEntryTree(repoId, branch, path string) (*model.Entry, error) {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(branch),
		ValidatePath(path))
	if err != nil {
		return nil, err
	}
	return index.ReadEntry(repoId, branch, path, model.Entry_TREE)
}

func (index *KVIndex) ReadEntryObject(repoId, branch, path string) (*model.Entry, error) {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(branch),
		ValidatePath(path))
	if err != nil {
		return nil, err
	}
	return index.ReadEntry(repoId, branch, path, model.Entry_OBJECT)
}

func (index *KVIndex) WriteFile(repoId, branch, path string, entry *model.Entry, obj *model.Object) error {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(branch),
		ValidatePath(path))
	if err != nil {
		return err
	}
	_, err = index.kv.RepoTransact(repoId, func(tx store.RepoOperations) (interface{}, error) {
		repo, err := tx.ReadRepo()
		if err != nil {
			return nil, err
		}
		err = tx.WriteObject(ident.Hash(obj), obj)
		if err != nil {
			index.log().WithError(err).Error("could not write object")
			return nil, err
		}
		err = writeEntryToWorkspace(tx, repo, branch, path, &model.WorkspaceEntry{
			Path:  path,
			Entry: entry,
		})
		if err != nil {
			index.log().WithError(err).Error("could not write workspace entry")
		}
		return nil, err
	})
	return err
}

func (index *KVIndex) WriteEntry(repoId, branch, path string, entry *model.Entry) error {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(branch),
		ValidatePath(path))
	if err != nil {
		return err
	}
	_, err = index.kv.RepoTransact(repoId, func(tx store.RepoOperations) (interface{}, error) {
		repo, err := tx.ReadRepo()
		if err != nil {
			return nil, err
		}
		err = writeEntryToWorkspace(tx, repo, branch, path, &model.WorkspaceEntry{
			Path:  path,
			Entry: entry,
		})
		if err != nil {
			index.log().WithError(err).Error("could not write workspace entry")
		}
		return nil, err
	})
	return err
}

func (index *KVIndex) WriteObject(repoId, branch, path string, object *model.Object) error {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(branch),
		ValidatePath(path))
	if err != nil {
		return err
	}
	timestamp := index.tsGenerator()
	_, err = index.kv.RepoTransact(repoId, func(tx store.RepoOperations) (interface{}, error) {
		addr := ident.Hash(object)
		err := tx.WriteObject(addr, object)
		if err != nil {
			return nil, err
		}
		repo, err := tx.ReadRepo()
		if err != nil {
			return nil, err
		}
		p := pth.New(path)
		err = writeEntryToWorkspace(tx, repo, branch, path, &model.WorkspaceEntry{
			Path: p.String(),
			Entry: &model.Entry{
				Name:      pth.New(path).Basename(),
				Address:   addr,
				Type:      model.Entry_OBJECT,
				Timestamp: timestamp,
				Size:      object.GetSize(),
				Checksum:  object.GetChecksum(),
			},
		})
		if err != nil {
			index.log().WithError(err).Error("could not write workspace entry")
		}
		return nil, err
	})
	return err
}

func (index *KVIndex) DeleteObject(repoId, branch, path string) error {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(branch),
		ValidatePath(path))
	if err != nil {
		return err
	}
	ts := index.tsGenerator()
	_, err = index.kv.RepoTransact(repoId, func(tx store.RepoOperations) (interface{}, error) {
		repo, err := tx.ReadRepo()
		if err != nil {
			return nil, err
		}
		/**
		handling 5 possible cases:
		* 1 object does not exist  - return error
		* 2 object exists only in workspace - remove from workspace
		* 3 object exists only in merkle - add tombstone
		* 4 object exists in workspace and in merkle - 2 + 3
		* 5 objects exists in merkle tombstone exists in workspace - return error
		*/
		notFoundCount := 0
		wsEntry, err := tx.ReadFromWorkspace(branch, path)
		if err != nil {
			if xerrors.Is(err, db.ErrNotFound) {
				notFoundCount += 1
			} else {
				return nil, err
			}
		}

		br, err := tx.ReadBranch(branch)
		if err != nil {
			return nil, err
		}
		root := br.GetWorkspaceRoot()
		m := merkle.New(root)
		merkleEntry, err := m.GetEntry(tx, path, model.Entry_OBJECT)
		if err != nil {
			if xerrors.Is(err, db.ErrNotFound) {
				notFoundCount += 1
			} else {
				return nil, err
			}
		}

		if notFoundCount == 2 {
			return nil, db.ErrNotFound
		}

		if wsEntry != nil {
			if wsEntry.Tombstone {
				return nil, db.ErrNotFound
			}
			err = tx.DeleteWorkspacePath(branch, path)
			if err != nil {
				return nil, err
			}
		}

		if merkleEntry != nil {
			err = writeEntryToWorkspace(tx, repo, branch, path, &model.WorkspaceEntry{
				Path: path,
				Entry: &model.Entry{
					Name:      pth.New(path).Basename(),
					Timestamp: ts,
					Type:      model.Entry_OBJECT,
				},
				Tombstone: true,
			})
			if err != nil {
				index.log().WithError(err).Error("could not write workspace tombstone")
			}
			return nil, err
		}
		return nil, nil
	})
	return err
}

func (index *KVIndex) ListBranchesByPrefix(repoId string, prefix string, amount int, after string) ([]*model.Branch, bool, error) {
	err := ValidateAll(
		ValidateRepoId(repoId))
	if err != nil {
		return nil, false, err
	}
	type result struct {
		hasMore bool
		results []*model.Branch
	}

	entries, err := index.kv.RepoTransact(repoId, func(tx store.RepoOperations) (interface{}, error) {
		// we're reading the repo to add it to this transaction's conflict range
		// but also to ensure it exists
		_, err := tx.ReadRepo()
		if err != nil {
			return nil, err
		}
		branches, hasMore, err := tx.ListBranches(prefix, amount, after)
		return &result{
			results: branches,
			hasMore: hasMore,
		}, err
	})
	if err != nil {
		index.log().WithError(err).Error("could not list branches")
		return nil, false, err
	}
	return entries.(*result).results, entries.(*result).hasMore, nil
}

func (index *KVIndex) ListObjectsByPrefix(repoId, ref, path, from string, results int, descend bool) ([]*model.Entry, bool, error) {
	log := index.log().WithFields(logging.Fields{
		"from":    from,
		"descend": descend,
		"results": results,
	})
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(ref),
		ValidatePath(path))
	if err != nil {
		return nil, false, err
	}
	type result struct {
		hasMore bool
		results []*model.Entry
	}
	entries, err := index.kv.RepoTransact(repoId, func(tx store.RepoOperations) (interface{}, error) {
		_, err := tx.ReadRepo()
		if err != nil {
			return nil, err
		}

		reference, err := resolveRef(tx, ref)
		if err != nil {
			return nil, err
		}

		var root string
		if reference.isBranch {
			err := partialCommit(tx, reference.branch.GetName()) // block on this since we traverse the tree immediately after
			if err != nil {
				return nil, err
			}
			reference.branch, err = tx.ReadBranch(reference.branch.Name)
			if err != nil {
				return nil, err
			}
			root = reference.branch.GetWorkspaceRoot()
		} else {
			root = reference.commit.GetTree()
		}

		tree := merkle.New(root)
		res, hasMore, err := tree.PrefixScan(tx, path, from, results, descend)
		if err != nil {
			log.WithError(err).Error("could not scan tree")
			return nil, err
		}
		return &result{hasMore, res}, nil
	})
	if err != nil {
		return nil, false, err
	}
	return entries.(*result).results, entries.(*result).hasMore, nil
}

func (index *KVIndex) ResetBranch(repoId, branch string) error {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(branch))
	if err != nil {
		return err
	}
	// clear workspace, set branch workspace root back to commit root
	_, err = index.kv.RepoTransact(repoId, func(tx store.RepoOperations) (interface{}, error) {
		err := tx.ClearWorkspace(branch)
		if err != nil {
			return nil, err
		}
		branchData, err := tx.ReadBranch(branch)
		if err != nil {
			return nil, err
		}
		gc(tx, branchData.GetWorkspaceRoot())
		branchData.WorkspaceRoot = branchData.GetCommitRoot()
		return nil, tx.WriteBranch(branch, branchData)
	})
	if err != nil {
		index.log().WithError(err).Error("could not reset branch")
	}
	return err
}

func (index *KVIndex) CreateBranch(repoId, branch, ref string) (*model.Branch, error) {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(ref),
		ValidateRef(branch))
	if err != nil {
		return nil, err
	}
	branchData, err := index.kv.RepoTransact(repoId, func(tx store.RepoOperations) (interface{}, error) {
		// ensure it doesn't exist yet
		_, err := tx.ReadBranch(branch)
		if err != nil && !xerrors.Is(err, db.ErrNotFound) {
			index.log().WithError(err).Error("could not read branch")
			return nil, err
		} else if err == nil {
			return nil, errors.ErrBranchAlreadyExists
		}
		// read resolve reference
		reference, err := resolveRef(tx, ref)
		if err != nil {
			return nil, xerrors.Errorf("could not read ref: %w", err)
		}
		branchData := &model.Branch{
			Name:          branch,
			Commit:        reference.commit.GetAddress(),
			CommitRoot:    reference.commit.GetTree(),
			WorkspaceRoot: reference.commit.GetTree(),
		}
		return branchData, tx.WriteBranch(branch, branchData)
	})
	if err != nil {
		index.log().WithError(err).WithField("ref", ref).Error("could not create branch")
		return nil, err
	}
	return branchData.(*model.Branch), nil
}

func (index *KVIndex) GetBranch(repoId, branch string) (*model.Branch, error) {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(branch))
	if err != nil {
		return nil, err
	}
	brn, err := index.kv.RepoReadTransact(repoId, func(tx store.RepoReadOnlyOperations) (i interface{}, err error) {
		return tx.ReadBranch(branch)
	})
	if err != nil {
		return nil, err
	}
	return brn.(*model.Branch), nil
}

func doCommitUpdates(tx store.RepoOperations, branchData *model.Branch, committer, message string, parents []string, metadata map[string]string, ts int64) (interface{}, error) {
	commit := &model.Commit{
		Tree:      branchData.GetWorkspaceRoot(),
		Parents:   parents,
		Committer: committer,
		Message:   message,
		Timestamp: ts,
		Metadata:  metadata,
	}
	commitAddr := ident.Hash(commit)
	commit.Address = commitAddr
	err := tx.WriteCommit(commitAddr, commit)
	if err != nil {
		return nil, err
	}
	branchData.Commit = commitAddr
	branchData.CommitRoot = commit.GetTree()
	branchData.WorkspaceRoot = commit.GetTree()
	return commit, tx.WriteBranch(branchData.Name, branchData)
}

func (index *KVIndex) Commit(repoId, branch, message, committer string, metadata map[string]string) (*model.Commit, error) {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(branch),
		ValidateCommitMessage(message))
	if err != nil {
		return nil, err
	}
	ts := index.tsGenerator()
	commit, err := index.kv.RepoTransact(repoId, func(tx store.RepoOperations) (interface{}, error) {
		err := partialCommit(tx, branch)
		if err != nil {
			return nil, err
		}
		branchData, err := tx.ReadBranch(branch)
		if err != nil {
			return nil, err
		}
		return doCommitUpdates(tx, branchData, committer, message, []string{branchData.GetCommit()}, metadata, ts)
	})
	if err != nil {
		return nil, err
	}
	return commit.(*model.Commit), nil
}

func (index *KVIndex) GetCommit(repoId, commitId string) (*model.Commit, error) {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateCommitID(commitId))
	if err != nil {
		return nil, err
	}
	commit, err := index.kv.RepoReadTransact(repoId, func(tx store.RepoReadOnlyOperations) (i interface{}, err error) {
		return tx.ReadCommit(commitId)
	})
	if err != nil {
		return nil, err
	}
	return commit.(*model.Commit), nil
}

func (index *KVIndex) GetCommitLog(repoId, fromCommitId string, results int, after string) ([]*model.Commit, bool, error) {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateCommitID(fromCommitId),
		ValidateOrEmpty(ValidateCommitID, after))

	type result struct {
		hasMore bool
		results []*model.Commit
	}
	if err != nil {
		return nil, false, err
	}
	res, err := index.kv.RepoReadTransact(repoId, func(tx store.RepoReadOnlyOperations) (i interface{}, err error) {
		commits, hasMore, err := dag.BfsScan(tx, fromCommitId, results, after)
		return &result{hasMore, commits}, err
	})
	if err != nil {
		index.log().WithError(err).WithField("from", fromCommitId).Error("could not read commits")
		return nil, false, err
	}
	return res.(*result).results, res.(*result).hasMore, nil
}

func (index *KVIndex) DeleteBranch(repoId, branch string) error {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(branch))
	if err != nil {
		return err
	}
	_, err = index.kv.RepoTransact(repoId, func(tx store.RepoOperations) (interface{}, error) {
		branchData, err := tx.ReadBranch(branch)
		if err != nil {
			return nil, err
		}
		err = tx.ClearWorkspace(branch)
		if err != nil {
			index.log().WithError(err).Error("could not clear workspace")
			return nil, err
		}
		gc(tx, branchData.GetWorkspaceRoot()) // changes are destroyed here
		err = tx.DeleteBranch(branch)
		if err != nil {
			index.log().WithError(err).Error("could not delete branch")
		}
		return nil, err
	})
	return err
}

func (index *KVIndex) DiffWorkspace(repoId, branch string) (merkle.Differences, error) {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(branch))
	if err != nil {
		return nil, err
	}
	res, err := index.kv.RepoTransact(repoId, func(tx store.RepoOperations) (i interface{}, err error) {
		err = partialCommit(tx, branch) // ensure all changes are reflected in the tree
		if err != nil {
			return nil, err
		}
		branch, err := tx.ReadBranch(branch)
		if err != nil {
			return nil, err
		}

		diff, err := merkle.Diff(tx,
			merkle.New(branch.GetWorkspaceRoot()),
			merkle.New(branch.GetCommitRoot()),
			merkle.New(branch.GetCommitRoot()))
		if err != nil {
			index.log().WithError(err).WithField("branch", branch).Error("diff workspace failed")
		}
		return diff, err
	})
	if err != nil {
		return nil, err
	}
	return res.(merkle.Differences), nil
}

func doDiff(tx store.RepoReadOnlyOperations, repoId, leftRef, rightRef string, isMerge bool, index *KVIndex) (merkle.Differences, error) {

	lRef, err := resolveRef(tx, leftRef)
	if err != nil {
		index.log().WithError(err).WithField("ref", leftRef).Error("could not resolve left ref")
		return nil, errors.ErrBranchNotFound
	}

	rRef, err := resolveRef(tx, rightRef)
	if err != nil {
		index.log().WithError(err).WithField("ref", rRef).Error("could not resolve right ref")
		return nil, errors.ErrBranchNotFound
	}

	commonCommits, err := dag.FindLowestCommonAncestor(tx, lRef.commit.GetAddress(), rRef.commit.GetAddress())
	if err != nil {
		index.log().WithField("left", lRef).WithField("right", rRef).WithError(err).Error("could not find common commit")
		return nil, errors.ErrNoMergeBase
	}
	if commonCommits == nil {
		index.log().WithField("left", lRef).WithField("right", rRef).Error("no common merge base found")
		return nil, errors.ErrNoMergeBase
	}

	leftTree := lRef.commit.GetTree()
	if lRef.isBranch && !isMerge {
		leftTree = lRef.branch.GetWorkspaceRoot()
	}
	rightTree := rRef.commit.GetTree()

	diff, err := merkle.Diff(tx,
		merkle.New(leftTree),
		merkle.New(rightTree),
		merkle.New(commonCommits.GetTree()))
	if err != nil {
		index.log().WithField("left", lRef).WithField("right", rRef).WithError(err).Error("could not calculate diff")
	}
	return diff, err
}

func (index *KVIndex) Diff(repoId, leftRef, rightRef string) (merkle.Differences, error) {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(leftRef),
		ValidateRef(rightRef))
	if err != nil {
		return nil, err
	}
	res, err := index.kv.RepoReadTransact(repoId, func(tx store.RepoReadOnlyOperations) (i interface{}, err error) {

		return doDiff(tx, repoId, leftRef, rightRef, false, index)
	})
	if err != nil {
		return nil, err
	}
	return res.(merkle.Differences), nil
}

func (index *KVIndex) RevertCommit(repoId, branch, commit string) error {
	log := index.log().WithFields(logging.Fields{
		"branch": branch,
		"commit": commit,
	})
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(branch),
		ValidateCommitID(commit))
	if err != nil {
		return err
	}
	_, err = index.kv.RepoTransact(repoId, func(tx store.RepoOperations) (interface{}, error) {
		err := tx.ClearWorkspace(branch)
		if err != nil {
			log.WithError(err).Error("could not revert commit")
			return nil, err
		}
		commitData, err := tx.ReadCommit(commit)
		if err != nil {
			return nil, err
		}
		branchData, err := tx.ReadBranch(branch)
		if err != nil {
			return nil, err
		}
		gc(tx, branchData.GetWorkspaceRoot())
		branchData.Commit = commit
		branchData.CommitRoot = commitData.GetTree()
		branchData.WorkspaceRoot = commitData.GetTree()
		err = tx.WriteBranch(branch, branchData)
		if err != nil {
			log.WithError(err).Error("could not write branch")
		}
		return nil, err
	})
	return err
}

func (index *KVIndex) revertPath(repoId, branch, path string, typ model.Entry_Type) error {
	log := index.log().WithFields(logging.Fields{
		"branch": branch,
		"path":   path,
	})
	_, err := index.kv.RepoTransact(repoId, func(tx store.RepoOperations) (interface{}, error) {
		p := pth.New(path)
		if p.IsRoot() {
			return nil, index.ResetBranch(repoId, branch)
		}

		err := partialCommit(tx, branch)
		if err != nil {
			log.WithError(err).Error("could not partially commit")
			return nil, err
		}
		branchData, err := tx.ReadBranch(branch)
		if err != nil {
			return nil, err
		}
		workspaceMerkle := merkle.New(branchData.GetWorkspaceRoot())
		commitMerkle := merkle.New(branchData.GetCommitRoot())
		var workspaceEntry *model.WorkspaceEntry
		commitEntry, err := commitMerkle.GetEntry(tx, path, typ)
		if err != nil {
			if xerrors.Is(err, db.ErrNotFound) {
				// remove all changes under path
				pathEntry, err := workspaceMerkle.GetEntry(tx, path, typ)
				if err != nil {
					return nil, err
				}
				workspaceEntry = &model.WorkspaceEntry{
					Path:      path,
					Entry:     pathEntry,
					Tombstone: true,
				}
			} else {
				log.WithError(err).Error("could not get entry")
				return nil, err
			}
		} else {
			workspaceEntry = &model.WorkspaceEntry{
				Path:  path,
				Entry: commitEntry,
			}
		}
		commitEntries := []*model.WorkspaceEntry{workspaceEntry}
		workspaceMerkle, err = workspaceMerkle.Update(tx, commitEntries)
		if err != nil {
			log.WithError(err).Error("could not update Merkle tree")
			return nil, err
		}

		// update branch workspace pointer to point at new workspace
		err = tx.WriteBranch(branch, &model.Branch{
			Name:          branch,
			Commit:        branchData.GetCommit(),
			CommitRoot:    branchData.GetCommitRoot(),
			WorkspaceRoot: workspaceMerkle.Root(),
		})

		if err != nil {
			log.WithError(err).Error("could not write branch")
		}
		return nil, err
	})
	return err
}

func (index *KVIndex) RevertPath(repoId, branch, path string) error {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(branch),
		ValidatePath(path))
	if err != nil {
		return err
	}
	return index.revertPath(repoId, branch, path, model.Entry_TREE)
}

func (index *KVIndex) RevertObject(repoId, branch, path string) error {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(branch),
		ValidatePath(path))
	if err != nil {
		return err
	}
	return index.revertPath(repoId, branch, path, model.Entry_OBJECT)
}

func (index *KVIndex) Merge(repoId, source, destination, userId string) (merkle.Differences, error) {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(source),
		ValidateRef(destination))
	if err != nil {
		return nil, err
	}
	ts := index.tsGenerator()
	var mergeOperations merkle.Differences
	_, err = index.kv.RepoTransact(repoId, func(tx store.RepoOperations) (interface{}, error) {
		// check that destination has no uncommitted changes
		destinationBranch, err := tx.ReadBranch(destination)
		if err != nil {
			index.log().WithError(err).WithField("destination", destination).Warn(" branch " + destination + " not found")
			return nil, errors.ErrBranchNotFound
		}
		l, err := tx.ListWorkspace(destination)
		if err != nil {
			index.log().WithError(err).WithField("destination", destination).Warn(" branch " + destination + " workspace not found")
			return nil, err
		}
		if destinationBranch.GetCommitRoot() != destinationBranch.GetWorkspaceRoot() || len(l) > 0 {
			return nil, errors.ErrDestinationNotCommitted
		}
		// compute difference
		df, err := doDiff(tx, repoId, source, destination, true, index)
		if err != nil {
			return nil, err
		}
		var isConflict bool
		for _, dif := range df {
			if dif.Direction == merkle.DifferenceDirectionConflict {
				isConflict = true
			}
			if dif.Direction != merkle.DifferenceDirectionRight {
				mergeOperations = append(mergeOperations, dif)
			}
		}
		if isConflict {
			return nil, errors.ErrMergeConflict
		}
		// update destination with source changes
		var wsEntries []*model.WorkspaceEntry
		sourceBranch, err := tx.ReadBranch(source)
		if err != nil {
			index.log().WithError(err).Fatal("failed reading source branch\n") // failure to read a branch that was read before fatal
			return nil, err
		}
		for _, dif := range mergeOperations {
			var e *model.Entry
			m := merkle.New(sourceBranch.GetWorkspaceRoot())
			if dif.Type != merkle.DifferenceTypeRemoved {
				e, err = m.GetEntry(tx, dif.Path, dif.PathType)
				if err != nil {
					index.log().WithError(err).Fatal("failed reading entry\n")
					return nil, err
				}
			} else {
				e = new(model.Entry)
				p := strings.Split(dif.Path, "/")
				e.Name = p[len(p)-1]
				e.Type = dif.PathType
			}
			w := new(model.WorkspaceEntry)
			w.Entry = e
			w.Path = dif.Path
			w.Tombstone = (dif.Type == merkle.DifferenceTypeRemoved)
			wsEntries = append(wsEntries, w)
		}

		desinationRoot := merkle.New(destinationBranch.GetCommitRoot())
		newRoot, err := desinationRoot.Update(tx, wsEntries)
		if err != nil {
			index.log().WithError(err).Fatal("failed updating merge destination\n")
			return nil, errors.ErrMergeUpdateFailed
		}
		destinationBranch.CommitRoot = newRoot.Root()
		destinationBranch.WorkspaceRoot = newRoot.Root()
		parents := []string{destinationBranch.GetCommit(), sourceBranch.GetCommit()}
		commitMessage := "Merge branch " + source + " into " + destination
		doCommitUpdates(tx, destinationBranch, userId, commitMessage, parents, make(map[string]string), ts)

		return mergeOperations, nil

	})
	if err == nil || err == errors.ErrMergeConflict {
		return mergeOperations, err
	} else {
		return nil, err
	}
}

func (index *KVIndex) CreateRepo(repoId, bucketName, defaultBranch string) error {
	err := ValidateAll(
		ValidateRepoId(repoId))
	if err != nil {
		return err
	}

	creationDate := index.tsGenerator()

	repo := &model.Repo{
		RepoId:             repoId,
		BucketName:         bucketName,
		CreationDate:       creationDate,
		DefaultBranch:      defaultBranch,
		PartialCommitRatio: DefaultPartialCommitRatio,
	}

	// create repository, an empty commit and tree, and the default branch
	_, err = index.kv.RepoTransact(repoId, func(tx store.RepoOperations) (interface{}, error) {
		// make sure this repo doesn't already exist
		_, err := tx.ReadRepo()
		if err == nil {
			// couldn't verify this bucket doesn't yet exist
			return nil, errors.ErrRepoExists
		} else if !xerrors.Is(err, db.ErrNotFound) {
			index.log().WithError(err).Error("could not read repo")
			return nil, err // error reading the repo
		}

		err = tx.WriteRepo(repo)
		if err != nil {
			return nil, err
		}
		commit := &model.Commit{
			Tree:      ident.Empty(),
			Parents:   []string{},
			Timestamp: creationDate,
			Metadata:  make(map[string]string),
		}
		commitId := ident.Hash(commit)
		commit.Address = commitId
		err = tx.WriteCommit(commitId, commit)
		if err != nil {
			index.log().WithError(err).Error("could not write initial commit")
			return nil, err
		}
		err = tx.WriteBranch(repo.GetDefaultBranch(), &model.Branch{
			Name:          repo.GetDefaultBranch(),
			Commit:        commitId,
			CommitRoot:    commit.GetTree(),
			WorkspaceRoot: commit.GetTree(),
		})
		if err != nil {
			index.log().WithError(err).Error("could not write branch")
		}
		return nil, err
	})
	return err
}

func (index *KVIndex) ListRepos(amount int, after string) ([]*model.Repo, bool, error) {
	type result struct {
		repos   []*model.Repo
		hasMore bool
	}
	res, err := index.kv.ReadTransact(func(tx store.ClientReadOnlyOperations) (interface{}, error) {
		repos, hasMore, err := tx.ListRepos(amount, after)
		return &result{
			repos:   repos,
			hasMore: hasMore,
		}, err
	})
	if err != nil {
		index.log().WithError(err).Error("could not list repos")
		return nil, false, err
	}
	return res.(*result).repos, res.(*result).hasMore, nil
}

func (index *KVIndex) GetRepo(repoId string) (*model.Repo, error) {
	err := ValidateAll(
		ValidateRepoId(repoId))
	if err != nil {
		return nil, err
	}
	repo, err := index.kv.ReadTransact(func(tx store.ClientReadOnlyOperations) (interface{}, error) {
		return tx.ReadRepo(repoId)
	})
	if err != nil {
		return nil, err
	}
	return repo.(*model.Repo), nil
}

func (index *KVIndex) DeleteRepo(repoId string) error {
	err := ValidateAll(
		ValidateRepoId(repoId))
	if err != nil {
		return err
	}
	_, err = index.kv.Transact(func(tx store.ClientOperations) (interface{}, error) {
		_, err := tx.ReadRepo(repoId)
		if err != nil {
			return nil, err
		}
		err = tx.DeleteRepo(repoId)
		if err != nil {
			index.log().WithError(err).Error("could not delete repo")
			return nil, err
		}
		return nil, nil
	})
	return err
}

func (index *KVIndex) Tree(repoId, branch string) error {
	err := ValidateAll(
		ValidateRepoId(repoId),
		ValidateRef(branch))
	if err != nil {
		return err
	}
	_, err = index.kv.RepoTransact(repoId, func(tx store.RepoOperations) (interface{}, error) {
		err := partialCommit(tx, branch)
		if err != nil {
			return nil, err
		}
		_, err = tx.ReadRepo()
		if err != nil {
			return nil, err
		}
		r, err := tx.ReadBranch(branch)
		if err != nil {
			return nil, err
		}
		m := merkle.New(r.GetWorkspaceRoot())
		m.WalkAll(tx)
		return nil, nil
	})
	return err
}
