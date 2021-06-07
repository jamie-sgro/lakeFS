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


package io.lakefs.clients.api.model;

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

/**
 * StorageConfig
 */
@javax.annotation.Generated(value = "org.openapitools.codegen.languages.JavaClientCodegen")
public class StorageConfig {
  public static final String SERIALIZED_NAME_BLOCKSTORE_NAMESPACE_EXAMPLE = "blockstore_namespace_example";
  @SerializedName(SERIALIZED_NAME_BLOCKSTORE_NAMESPACE_EXAMPLE)
  private String blockstoreNamespaceExample;

  public static final String SERIALIZED_NAME_BLOCKSTORE_NAMESPACE_VALIDITY_REGEX = "blockstore_namespace_ValidityRegex";
  @SerializedName(SERIALIZED_NAME_BLOCKSTORE_NAMESPACE_VALIDITY_REGEX)
  private String blockstoreNamespaceValidityRegex;


  public StorageConfig blockstoreNamespaceExample(String blockstoreNamespaceExample) {
    
    this.blockstoreNamespaceExample = blockstoreNamespaceExample;
    return this;
  }

   /**
   * Get blockstoreNamespaceExample
   * @return blockstoreNamespaceExample
  **/
  @ApiModelProperty(required = true, value = "")

  public String getBlockstoreNamespaceExample() {
    return blockstoreNamespaceExample;
  }


  public void setBlockstoreNamespaceExample(String blockstoreNamespaceExample) {
    this.blockstoreNamespaceExample = blockstoreNamespaceExample;
  }


  public StorageConfig blockstoreNamespaceValidityRegex(String blockstoreNamespaceValidityRegex) {
    
    this.blockstoreNamespaceValidityRegex = blockstoreNamespaceValidityRegex;
    return this;
  }

   /**
   * Get blockstoreNamespaceValidityRegex
   * @return blockstoreNamespaceValidityRegex
  **/
  @ApiModelProperty(required = true, value = "")

  public String getBlockstoreNamespaceValidityRegex() {
    return blockstoreNamespaceValidityRegex;
  }


  public void setBlockstoreNamespaceValidityRegex(String blockstoreNamespaceValidityRegex) {
    this.blockstoreNamespaceValidityRegex = blockstoreNamespaceValidityRegex;
  }


  @Override
  public boolean equals(Object o) {
    if (this == o) {
      return true;
    }
    if (o == null || getClass() != o.getClass()) {
      return false;
    }
    StorageConfig storageConfig = (StorageConfig) o;
    return Objects.equals(this.blockstoreNamespaceExample, storageConfig.blockstoreNamespaceExample) &&
        Objects.equals(this.blockstoreNamespaceValidityRegex, storageConfig.blockstoreNamespaceValidityRegex);
  }

  @Override
  public int hashCode() {
    return Objects.hash(blockstoreNamespaceExample, blockstoreNamespaceValidityRegex);
  }

  @Override
  public String toString() {
    StringBuilder sb = new StringBuilder();
    sb.append("class StorageConfig {\n");
    sb.append("    blockstoreNamespaceExample: ").append(toIndentedString(blockstoreNamespaceExample)).append("\n");
    sb.append("    blockstoreNamespaceValidityRegex: ").append(toIndentedString(blockstoreNamespaceValidityRegex)).append("\n");
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

