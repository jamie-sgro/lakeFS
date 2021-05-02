/*
 * lakeFS API
 * lakeFS HTTP API
 *
 * The version of the OpenAPI document: 0.1.0
 * 
 *
 * NOTE: This class is auto generated by OpenAPI Generator (https://openapi-generator.tech).
 * https://openapi-generator.tech
 * Do not edit the class manually.
 */


package io.treeverse.lakefs.clients.api.model;

import java.util.Objects;
import java.util.Arrays;
import com.google.gson.TypeAdapter;
import com.google.gson.annotations.JsonAdapter;
import com.google.gson.annotations.SerializedName;
import com.google.gson.stream.JsonReader;
import com.google.gson.stream.JsonWriter;
import io.swagger.annotations.ApiModel;
import io.swagger.annotations.ApiModelProperty;
import java.io.IOException;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * ObjectStageCreation
 */
@javax.annotation.Generated(value = "org.openapitools.codegen.languages.JavaClientCodegen", date = "2021-05-02T11:17:14.862Z[GMT]")
public class ObjectStageCreation {
  public static final String SERIALIZED_NAME_PHYSICAL_ADDRESS = "physical_address";
  @SerializedName(SERIALIZED_NAME_PHYSICAL_ADDRESS)
  private String physicalAddress;

  public static final String SERIALIZED_NAME_CHECKSUM = "checksum";
  @SerializedName(SERIALIZED_NAME_CHECKSUM)
  private String checksum;

  public static final String SERIALIZED_NAME_SIZE_BYTES = "size_bytes";
  @SerializedName(SERIALIZED_NAME_SIZE_BYTES)
  private Long sizeBytes;

  public static final String SERIALIZED_NAME_METADATA = "metadata";
  @SerializedName(SERIALIZED_NAME_METADATA)
  private Map<String, String> metadata = null;


  public ObjectStageCreation physicalAddress(String physicalAddress) {
    
    this.physicalAddress = physicalAddress;
    return this;
  }

   /**
   * Get physicalAddress
   * @return physicalAddress
  **/
  @ApiModelProperty(required = true, value = "")

  public String getPhysicalAddress() {
    return physicalAddress;
  }


  public void setPhysicalAddress(String physicalAddress) {
    this.physicalAddress = physicalAddress;
  }


  public ObjectStageCreation checksum(String checksum) {
    
    this.checksum = checksum;
    return this;
  }

   /**
   * Get checksum
   * @return checksum
  **/
  @ApiModelProperty(required = true, value = "")

  public String getChecksum() {
    return checksum;
  }


  public void setChecksum(String checksum) {
    this.checksum = checksum;
  }


  public ObjectStageCreation sizeBytes(Long sizeBytes) {
    
    this.sizeBytes = sizeBytes;
    return this;
  }

   /**
   * Get sizeBytes
   * @return sizeBytes
  **/
  @ApiModelProperty(required = true, value = "")

  public Long getSizeBytes() {
    return sizeBytes;
  }


  public void setSizeBytes(Long sizeBytes) {
    this.sizeBytes = sizeBytes;
  }


  public ObjectStageCreation metadata(Map<String, String> metadata) {
    
    this.metadata = metadata;
    return this;
  }

  public ObjectStageCreation putMetadataItem(String key, String metadataItem) {
    if (this.metadata == null) {
      this.metadata = new HashMap<String, String>();
    }
    this.metadata.put(key, metadataItem);
    return this;
  }

   /**
   * Get metadata
   * @return metadata
  **/
  @javax.annotation.Nullable
  @ApiModelProperty(value = "")

  public Map<String, String> getMetadata() {
    return metadata;
  }


  public void setMetadata(Map<String, String> metadata) {
    this.metadata = metadata;
  }


  @Override
  public boolean equals(Object o) {
    if (this == o) {
      return true;
    }
    if (o == null || getClass() != o.getClass()) {
      return false;
    }
    ObjectStageCreation objectStageCreation = (ObjectStageCreation) o;
    return Objects.equals(this.physicalAddress, objectStageCreation.physicalAddress) &&
        Objects.equals(this.checksum, objectStageCreation.checksum) &&
        Objects.equals(this.sizeBytes, objectStageCreation.sizeBytes) &&
        Objects.equals(this.metadata, objectStageCreation.metadata);
  }

  @Override
  public int hashCode() {
    return Objects.hash(physicalAddress, checksum, sizeBytes, metadata);
  }

  @Override
  public String toString() {
    StringBuilder sb = new StringBuilder();
    sb.append("class ObjectStageCreation {\n");
    sb.append("    physicalAddress: ").append(toIndentedString(physicalAddress)).append("\n");
    sb.append("    checksum: ").append(toIndentedString(checksum)).append("\n");
    sb.append("    sizeBytes: ").append(toIndentedString(sizeBytes)).append("\n");
    sb.append("    metadata: ").append(toIndentedString(metadata)).append("\n");
    sb.append("}");
    return sb.toString();
  }

  /**
   * Convert the given object to string with each line indented by 4 spaces
   * (except the first line).
   */
  private String toIndentedString(Object o) {
    if (o == null) {
      return "null";
    }
    return o.toString().replace("\n", "\n    ");
  }

}

