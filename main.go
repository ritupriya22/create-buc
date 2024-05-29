package main
import (
   // "context"
    "time"
    "bytes"
    "crypto/tls"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "net/http"
    "github.com/gin-gonic/gin"
    _ "github.com/lib/pq"
   // "github.com/segmentio/kafka-go"
    "go.uber.org/zap"
    "log"
   // "os"
    "database/sql"
    _ "github.com/swaggo/gin-swagger"
    swaggerFiles "github.com/swaggo/files"
    // _ "/home/ritu/create-buc/docs"
)
var logger, _ = zap.NewProduction()
const (
    noobaaURL = "https://noobaa-mgmt-openshift-storage.apps.dev.jecs.jio.com/rpc/"
    authToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhY2NvdW50X2lkIjoiNjU0YjUwMDY0OTYyY2IwMDI5Nzk0YzM5Iiwic3lzdGVtX2lkIjoiNjU0YjUwMDY0OTYyY2IwMDI5Nzk0YzNhIiwicm9sZSI6ImFkbWluIiwiYXV0aG9yaXplZF9ieSI6Im5vb2JhYSIsImlhdCI6MTY5OTQzNDU2Nn0.kiyWP52DHio8Aimypj4o_vTV6QpYTqVUfC7SQYD6-Rc"
)
type RPCRequest struct {
    API       string      `json:"api"`
    Method    string      `json:"method"`
    Params    interface{} `json:"params,omitempty"`
    AuthToken string      `json:"auth_token"`
}
type RPCResponse struct {
    Reply interface{} `json:"reply"`
}
type Encryption struct {
    Algorithm    string      `json:"algorithm"`
    KeyId         string      `json:"kms_key_id"`
}
type CreateBucketParams struct {
    Bucket string `json:"bucket_name"`
    User   string `json:"user_id"`
    Key    string `json:"key"`
}
 
 
func makeRPCRequest(requestBody RPCRequest) (RPCResponse, error) {
    var response RPCResponse
    jsonValue, _ := json.Marshal(requestBody)
    // Create a custom HTTP client with a custom TLS configuration
    client := &http.Client{
        Transport: &http.Transport{
            TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // This disables certificate verification
        },
    }
    resp, err := client.Post(noobaaURL, "application/json", bytes.NewBuffer(jsonValue))
    if err!= nil {
        logger.Error("Failed to make RPC request", zap.Error(err))
        zap.L().Info("Failed to make RPC request", zap.Error(err), zap.Time("timestamp", time.Now()))
        return response, err
    }
    defer resp.Body.Close()
    body, _ := ioutil.ReadAll(resp.Body)
    err = json.Unmarshal(body, &response)
    if err!= nil {
        logger.Error("Failed to unmarshal response", zap.Error(err))
        zap.L().Info("Failed to unmarshal response", zap.Error(err), zap.Time("timestamp", time.Now()))
        return response, err
    }
    return response, nil
}
 
func putEncrypt(bucketName string, encryptionKey string) (RPCResponse, error) {
    encryption := Encryption{
        Algorithm :  "AES256" ,
        KeyId : encryptionKey,
    }
    requestBody := RPCRequest{
        API:       "bucket_api",
        Method:    "put_bucket_encryption",
        Params:    map[string]interface{}{"name": bucketName, "encryption": encryption},
        AuthToken: authToken,
    }
    response, err := makeRPCRequest(requestBody)
    if err!= nil {
        zap.L().Info("Error putting bucket with encryption", zap.Error(err), zap.Time("timestamp", time.Now()), zap.String("bucket_name", bucketName))
        logger.Error("Error putting bucket with encryption", zap.Error(err), zap.String("bucket_name", bucketName))
        return response, err
    }
    return response, nil
}

func createBucket(bucketName string, userId string, encryptionKey string) (RPCResponse, error) {
    bucketClaim := map[string]string{ "bucket_class": "noobaa-default-bucket-class", "namespace": "noobaa", }
    requestBody := RPCRequest{
        API:       "bucket_api",
        Method:    "create_bucket",
        Params:    map[string]interface{}{"name": bucketName, "bucket_claim": bucketClaim},
        AuthToken: authToken,
    }
    response, err := makeRPCRequest(requestBody)
    if err!= nil {
        zap.L().Info("Error creating bucket", zap.Error(err), zap.Time("timestamp", time.Now()), zap.String("bucket_name", bucketName))
        logger.Error("Error creating bucket", zap.Error(err), zap.String("bucket_name", bucketName))
        return response, err
    }
    /*err = addUserLabel(userId, bucketName)
    if err!=nil {
        return response, err
    }*/
    host := "noobaa-db-pg.openshift-storage.svc.cluster.local"
    port := 5432
    user := "noobaa"
    password := "1OzXjadKG5h0zQ=="
    dbName := "nbcore"
    psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
            host, port, user, password, dbName)
    db, err := sql.Open("postgres", psqlInfo)
    if err!= nil {
        log.Fatal(err)
    }
    defer db.Close()
    err = db.Ping()
    if err!= nil {
        log.Fatal(err)
    }
    sqlUpdateStatement := fmt.Sprintf(`
    UPDATE public.buckets
    SET data = jsonb_set(data, '{%s}', '"%s"', true)
    WHERE data ->> 'name' = $1
    RETURNING data;
    `, "userId", userId)
    rowUpdate := db.QueryRow(sqlUpdateStatement, bucketName)
    var updatedData string
    err = rowUpdate.Scan(&updatedData)
    if err!= nil {
      //      c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        zap.L().Info("Error adding user label to bucket", zap.Error(err), zap.Time("timestamp", time.Now()), zap.String("bucket_name", bucketName), zap.String("user_id", userId))
        logger.Error("Error adding user label to bucket", zap.Error(err), zap.String("bucket_name", bucketName), zap.String("user_id", userId))
        _, errd := deleteBucket(bucketName)
        zap.L().Info("Error reverting bucket creation", zap.Error(errd), zap.Time("timestamp", time.Now()), zap.String("bucket_name", bucketName), zap.String("user_id", userId))
        logger.Error("Error reverting bucket creation", zap.Error(errd), zap.String("bucket_name", bucketName), zap.String("user_id", userId))
        return response, err
        }
    res, err := putEncrypt(bucketName, encryptionKey)
    if err!=nil {
        return res, err
        _, errd :=deleteBucket(bucketName)
        zap.L().Info("Error reverting bucket creation", zap.Error(errd), zap.Time("timestamp", time.Now()), zap.String("bucket_name", bucketName), zap.String("user_id", userId))
        logger.Error("Error reverting bucket creation", zap.Error(errd), zap.String("bucket_name", bucketName), zap.String("user_id", userId))
    }
    return response, nil
}
 
func deleteBucket(bucketName string) (RPCResponse, error) {
    requestBody := RPCRequest{
        API:       "bucket_api",
        Method:    "delete_bucket",
        Params:    map[string]interface{}{"bucket": bucketName},
        AuthToken: authToken,
    }
    response, err := makeRPCRequest(requestBody)
    if err!=nil {
    return response, err
    }
    return response, nil
}

func main() {
    router := gin.Default()
    // Define routes for your application
    router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
    // @Summary Create a new bucket
    // @Description Create a new bucket with the given parameters
    // @Tags buckets
    // @Accept  json
    // @Produce  json
    // @Param params body CreateBucketParams true "CreateBucketParams"
    // @Success 200 {object} map[string]string
    // @Router /createBucket [post]
    router.POST("/createBucket", func(c *gin.Context) {
        var params CreateBucketParams
        if err := c.BindJSON(&params); err!= nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
            return
        }
        bucketName := params.Bucket
        userId := params.User
        encryptionKey := params.Key
        _, err := createBucket(bucketName, userId, encryptionKey)
        if err!= nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Unable to create bucket: %s", err.Error())})
            zap.L().Info("Error creating bucket-client", zap.Error(err), zap.Time("timestamp", time.Now()), zap.String("bucket_name", bucketName))
            logger.Error("Error creating bucket-client", zap.Error(err), zap.String("bucket_name", bucketName), zap.String("user_id", userId), zap.String("encryption_key", encryptionKey))
            return
        }
        timestamp := time.Now() // Example format, adjust as needed
        creationTime := timestamp.Format(time.RFC3339)
        // Custom message including static text and dynamic timestamp
        c.JSON(200, gin.H{
            "message": "Bucket Created",
            "date":    creationTime,
        })
    })
 
    router.Run(":5000")
}
 
