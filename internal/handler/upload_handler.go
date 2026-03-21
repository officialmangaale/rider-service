package handler

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
)

type UploadHandler struct {
	s3Client *s3.Client
	bucket   string
	region   string
}

func NewUploadHandler() *UploadHandler {
	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		bucket = "mangaale-prod"
	}

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "ap-south-1"
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		fmt.Printf("Warning: Failed to load AWS config: %v\n", err)
	}

	client := s3.NewFromConfig(cfg)

	return &UploadHandler{
		s3Client: client,
		bucket:   bucket,
		region:   region,
	}
}

func (h *UploadHandler) HandleUpload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		dto.ValidationError(c, "File is required")
		return
	}

	src, err := file.Open()
	if err != nil {
		dto.InternalError(c, "Failed to open file")
		return
	}
	defer src.Close()

	ext := filepath.Ext(file.Filename)
	if ext == "" {
		ext = ".bin"
	}
	
	newFileName := fmt.Sprintf("riders/%s%s", uuid.New().String(), strings.ToLower(ext))

	_, err = h.s3Client.PutObject(c.Request.Context(), &s3.PutObjectInput{
		Bucket: aws.String(h.bucket),
		Key:    aws.String(newFileName),
		Body:   src,
	})

	if err != nil {
		fmt.Printf("S3 upload error: %v\n", err)
		dto.InternalError(c, "Failed to upload file")
		return
	}

	fileURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", h.bucket, h.region, newFileName)
	
	dto.Success(c, http.StatusOK, "File uploaded successfully", gin.H{
		"file_url": fileURL,
	})
}
