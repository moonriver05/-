package storage

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	cos "github.com/tencentyun/cos-go-sdk-v5"
)

type AvatarStore interface {
	Configured() bool
	Put(context.Context, string, []byte) error
	PublicURL(string) (string, error)
}
type COS struct {
	client                             *cos.Client
	secretID, secretKey, token, prefix string
}

var bucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*-[0-9]+$`)
var regionPattern = regexp.MustCompile(`^[a-z]+-[a-z]+(?:-[0-9]+)?$`)
var keyPattern = regexp.MustCompile(`^[0-9]+/[a-f0-9]{32}\.jpg$`)

func FromEnv() *COS {
	s := &COS{secretID: os.Getenv("COS_SECRET_ID"), secretKey: os.Getenv("COS_SECRET_KEY"), token: os.Getenv("COS_SESSION_TOKEN"), prefix: "mathwiki/avatars/"}
	bucket, region := strings.TrimSpace(os.Getenv("COS_BUCKET")), strings.TrimSpace(os.Getenv("COS_REGION"))
	if !bucketPattern.MatchString(bucket) || !regionPattern.MatchString(region) || s.secretID == "" || s.secretKey == "" {
		return s
	}
	endpoint, _ := url.Parse("https://" + bucket + ".cos." + region + ".myqcloud.com")
	s.client = cos.NewClient(&cos.BaseURL{BucketURL: endpoint}, &http.Client{Timeout: 20 * time.Second, Transport: &cos.AuthorizationTransport{SecretID: s.secretID, SecretKey: s.secretKey, SessionToken: s.token}})
	return s
}
func (s *COS) Configured() bool { return s != nil && s.client != nil }
func (s *COS) Put(ctx context.Context, key string, data []byte) error {
	if !s.Configured() || !keyPattern.MatchString(key) {
		return errors.New("COS avatar storage unavailable")
	}
	options := &cos.ObjectPutOptions{ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{ContentType: "image/jpeg", CacheControl: "public, max-age=86400"}}
	_, err := s.client.Object.Put(ctx, s.prefix+key, bytes.NewReader(data), options)
	return err
}
func (s *COS) PublicURL(key string) (string, error) {
	if !s.Configured() || !keyPattern.MatchString(key) {
		return "", errors.New("COS avatar storage unavailable")
	}
	return s.client.BaseURL.BucketURL.String() + "/" + s.prefix + key, nil
}
