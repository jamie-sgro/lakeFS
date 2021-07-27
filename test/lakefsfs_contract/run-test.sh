#!/bin/bash -ex
REPOSITORY=${REPOSITORY:-example}


docker-compose exec -T lakefs lakectl repo create "lakefs://${REPOSITORY}" ${STORAGE_NAMESPACE} -d main

mvn test
l