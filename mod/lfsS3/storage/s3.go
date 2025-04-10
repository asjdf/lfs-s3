package storage

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"
)

type S3Storage struct {
	client         *s3.S3
	ExternalClient *s3.S3
	bucketName     string
}

type S3Config struct {
	ExternalEndpoint string `yaml:"externalEndpoint"`
	Endpoint         string `yaml:"endpoint"`
	AccessKeyID      string `yaml:"accessKeyID"`
	SecretAccessKey  string `yaml:"secretAccessKey"`
	BucketName       string `yaml:"bucketName"`
	Region           string `yaml:"region"`
	PathStyle        bool   `yaml:"pathStyle"`
}

func NewS3Storage(cfg S3Config) (*S3Storage, error) {
	s3Config := &aws.Config{
		Credentials:      credentials.NewStaticCredentials(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Endpoint:         aws.String(cfg.ExternalEndpoint),
		Region:           aws.String(cfg.Region),
		S3ForcePathStyle: aws.Bool(cfg.PathStyle),
	}
	newSession, err := session.NewSession(s3Config)
	if err != nil {
		return nil, errors.Wrap(err, "init session")
	}
	eClient := s3.New(newSession)

	client := eClient
	if cfg.Endpoint != "" {
		s3Config = &aws.Config{
			Credentials:      credentials.NewStaticCredentials(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
			Endpoint:         aws.String(cfg.Endpoint),
			Region:           aws.String(cfg.Region),
			S3ForcePathStyle: aws.Bool(cfg.PathStyle),
		}
		newSession, err = session.NewSession(s3Config)
		if err != nil {
			return nil, errors.Wrap(err, "init session")
		}
		client = s3.New(newSession)
	}

	return &S3Storage{
		client:         client,
		ExternalClient: eClient,
		bucketName:     cfg.BucketName,
	}, nil
}

func (s *S3Storage) ObjectExists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObjectWithContext(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (s *S3Storage) GetObjectDownloadURL(ctx context.Context, key string, expiresIn ...time.Duration) (string, error) {
	var client *s3.S3
	if s.ExternalClient != nil {
		client = s.ExternalClient
	} else {
		client = s.client
	}

	defaultExpiresIn := 24 * time.Hour
	if len(expiresIn) > 0 {
		defaultExpiresIn = expiresIn[0]
	}

	getReq, _ := client.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	return getReq.Presign(defaultExpiresIn)
}

// GetObjectUploadURL 生成对象的上传预签名URL
func (s *S3Storage) GetObjectUploadURL(ctx context.Context, key string, expiresIn ...time.Duration) (string, error) {
	var client *s3.S3
	if s.ExternalClient != nil {
		client = s.ExternalClient
	} else {
		client = s.client
	}

	defaultExpiresIn := 24 * time.Hour
	if len(expiresIn) > 0 {
		defaultExpiresIn = expiresIn[0]
	}

	putReq, _ := client.PutObjectRequest(&s3.PutObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	return putReq.Presign(defaultExpiresIn)
}
