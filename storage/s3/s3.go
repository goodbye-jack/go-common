package s3

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/goodbye-jack/go-common/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	s3EndpointKey          = "s3_endpoint"
	s3RegionKey            = "s3_region"
	s3AccessKeyKey         = "s3_access_key"
	s3SecretKeyKey         = "s3_secret_key"
	s3BucketKey            = "s3_bucket"
	s3UseSSLKey            = "s3_use_ssl"
	s3ForcePathStyleKey    = "s3_force_path_style"
	s3BasePrefixKey        = "s3_base_prefix"
	s3PublicKey            = "s3_public"
	s3SignExpireSecondsKey = "s3_sign_expire_seconds"
)

const (
	defaultRegion           = "us-east-1"
	defaultSignExpireSecond = 600
	maxDirLevel             = 2
	maxDirSegmentLen        = 32
)

var dirSegmentRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

type ParamsError struct {
	Params []string
}

func (e ParamsError) Error() string {
	return fmt.Sprintf("params %+v error", e.Params)
}

type Config struct {
	Endpoint          string
	Region            string
	AccessKey         string
	SecretKey         string
	Bucket            string
	UseSSL            bool
	ForcePathStyle    bool
	BasePrefix        string
	Public            bool
	SignExpireSeconds int
}

func LoadConfig() (Config, error) {
	cfg := Config{
		Endpoint:          strings.TrimSpace(config.GetConfigString(s3EndpointKey)),
		Region:            strings.TrimSpace(config.GetConfigString(s3RegionKey)),
		AccessKey:         strings.TrimSpace(config.GetConfigString(s3AccessKeyKey)),
		SecretKey:         config.GetConfigString(s3SecretKeyKey),
		Bucket:            strings.TrimSpace(config.GetConfigString(s3BucketKey)),
		UseSSL:            config.GetConfigBool(s3UseSSLKey),
		ForcePathStyle:    config.GetConfigBool(s3ForcePathStyleKey),
		BasePrefix:        strings.TrimSpace(config.GetConfigString(s3BasePrefixKey)),
		Public:            config.GetConfigBool(s3PublicKey),
		SignExpireSeconds: config.GetConfigInt(s3SignExpireSecondsKey),
	}
	cfg = normalizeConfig(cfg)
	return cfg, validateConfig(cfg)
}

func normalizeConfig(cfg Config) Config {
	if cfg.Region == "" {
		cfg.Region = defaultRegion
	}
	if cfg.SignExpireSeconds <= 0 {
		cfg.SignExpireSeconds = defaultSignExpireSecond
	}
	cfg.BasePrefix = strings.Trim(cfg.BasePrefix, "/")
	cfg.Endpoint = strings.TrimRight(cfg.Endpoint, "/")
	cfg.Bucket = strings.Trim(cfg.Bucket, "/")
	return cfg
}

func validateConfig(cfg Config) error {
	missing := []string{}
	if cfg.Endpoint == "" {
		missing = append(missing, s3EndpointKey)
	}
	if cfg.AccessKey == "" {
		missing = append(missing, s3AccessKeyKey)
	}
	if cfg.SecretKey == "" {
		missing = append(missing, s3SecretKeyKey)
	}
	if cfg.Bucket == "" {
		missing = append(missing, s3BucketKey)
	}
	if len(missing) > 0 {
		return ParamsError{Params: missing}
	}
	return nil
}

type Client struct {
	cfg          Config
	client       *minio.Client
	endpointHost string
	secure       bool
}

func NewFromConfig() (*Client, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	return New(cfg)
}

func New(cfg Config) (*Client, error) {
	cfg = normalizeConfig(cfg)
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	host, secure, err := normalizeEndpoint(cfg.Endpoint, cfg.UseSSL)
	if err != nil {
		return nil, err
	}
	if !cfg.ForcePathStyle && shouldForcePathStyle(host) {
		cfg.ForcePathStyle = true
	}
	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: secure,
		Region: cfg.Region,
	}
	if cfg.ForcePathStyle {
		opts.BucketLookup = minio.BucketLookupPath
	}
	mc, err := minio.New(host, opts)
	if err != nil {
		return nil, err
	}
	return &Client{
		cfg:          cfg,
		client:       mc,
		endpointHost: host,
		secure:       secure,
	}, nil
}

func (c *Client) Config() Config {
	return c.cfg
}

func (c *Client) BuildObjectKey(dir, fileID, ext string) (string, string, string, string, error) {
	return BuildObjectKey(c.cfg.BasePrefix, dir, fileID, ext)
}

func (c *Client) Upload(ctx context.Context, objectKey string, r io.Reader, size int64, contentType string) (string, error) {
	if strings.TrimSpace(objectKey) == "" {
		return "", fmt.Errorf("empty object key")
	}
	if size < 0 {
		return "", fmt.Errorf("invalid size")
	}
	opts := minio.PutObjectOptions{ContentType: contentType}
	_, err := c.client.PutObject(ctx, c.cfg.Bucket, objectKey, r, size, opts)
	if err != nil {
		return "", err
	}
	if c.cfg.Public {
		return c.ObjectURL(objectKey), nil
	}
	return "", nil
}

func (c *Client) Download(ctx context.Context, objectKey string) (*minio.Object, error) {
	if strings.TrimSpace(objectKey) == "" {
		return nil, fmt.Errorf("empty object key")
	}
	return c.client.GetObject(ctx, c.cfg.Bucket, objectKey, minio.GetObjectOptions{})
}

func (c *Client) Delete(ctx context.Context, objectKey string) error {
	if strings.TrimSpace(objectKey) == "" {
		return fmt.Errorf("empty object key")
	}
	return c.client.RemoveObject(ctx, c.cfg.Bucket, objectKey, minio.RemoveObjectOptions{})
}

func (c *Client) Stat(ctx context.Context, objectKey string) (minio.ObjectInfo, error) {
	if strings.TrimSpace(objectKey) == "" {
		return minio.ObjectInfo{}, fmt.Errorf("empty object key")
	}
	return c.client.StatObject(ctx, c.cfg.Bucket, objectKey, minio.StatObjectOptions{})
}

func (c *Client) PresignGet(ctx context.Context, objectKey string, expireSeconds int) (string, error) {
	if strings.TrimSpace(objectKey) == "" {
		return "", fmt.Errorf("empty object key")
	}
	if expireSeconds <= 0 {
		expireSeconds = c.cfg.SignExpireSeconds
	}
	if expireSeconds <= 0 {
		expireSeconds = defaultSignExpireSecond
	}
	u, err := c.client.PresignedGetObject(ctx, c.cfg.Bucket, objectKey, time.Duration(expireSeconds)*time.Second, url.Values{})
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (c *Client) ObjectURL(objectKey string) string {
	objectKey = strings.TrimPrefix(objectKey, "/")
	base := c.endpointURL()
	if c.cfg.ForcePathStyle {
		return fmt.Sprintf("%s/%s/%s", base, c.cfg.Bucket, objectKey)
	}
	return fmt.Sprintf("%s/%s", c.bucketHostURL(), objectKey)
}

func (c *Client) endpointURL() string {
	scheme := "http"
	if c.secure {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, c.endpointHost)
}

func (c *Client) bucketHostURL() string {
	scheme := "http"
	if c.secure {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s.%s", scheme, c.cfg.Bucket, c.endpointHost)
}

func NormalizeDir(dir string) (string, string, string, error) {
	clean := strings.TrimSpace(dir)
	if clean == "" {
		return "", "", "", nil
	}
	if strings.Contains(clean, `\`) || strings.Contains(clean, ":") {
		return "", "", "", fmt.Errorf("invalid dir")
	}
	clean = strings.Trim(clean, "/")
	if clean == "" {
		return "", "", "", nil
	}
	parts := strings.Split(clean, "/")
	if len(parts) > maxDirLevel {
		return "", "", "", fmt.Errorf("dir level overflow")
	}
	for _, p := range parts {
		if p == "" {
			return "", "", "", fmt.Errorf("invalid dir")
		}
		if len(p) > maxDirSegmentLen || !dirSegmentRe.MatchString(p) {
			return "", "", "", fmt.Errorf("invalid dir")
		}
	}
	lv1 := parts[0]
	lv2 := ""
	if len(parts) > 1 {
		lv2 = parts[1]
	}
	return strings.Join(parts, "/"), lv1, lv2, nil
}

func BuildObjectKey(basePrefix, dir, fileID, ext string) (string, string, string, string, error) {
	if strings.TrimSpace(fileID) == "" {
		return "", "", "", "", fmt.Errorf("empty file id")
	}
	cleanDir, lv1, lv2, err := NormalizeDir(dir)
	if err != nil {
		return "", "", "", "", err
	}
	basePrefix = strings.Trim(basePrefix, "/")
	ext = strings.TrimSpace(ext)
	if strings.HasPrefix(ext, ".") {
		ext = strings.TrimPrefix(ext, ".")
	}
	name := fileID
	if ext != "" {
		name = fileID + "." + ext
	}
	parts := make([]string, 0, 3)
	if basePrefix != "" {
		parts = append(parts, basePrefix)
	}
	if cleanDir != "" {
		parts = append(parts, cleanDir)
	}
	parts = append(parts, name)
	return strings.Join(parts, "/"), cleanDir, lv1, lv2, nil
}

func normalizeEndpoint(endpoint string, useSSL bool) (string, bool, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", false, fmt.Errorf("empty endpoint")
	}
	if strings.Contains(endpoint, "://") {
		u, err := url.Parse(endpoint)
		if err != nil {
			return "", false, err
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return "", false, fmt.Errorf("unsupported scheme")
		}
		secure := u.Scheme == "https"
		host := strings.TrimSpace(u.Host)
		if host == "" {
			return "", false, fmt.Errorf("invalid endpoint")
		}
		return host, secure, nil
	}
	return strings.TrimRight(endpoint, "/"), useSSL, nil
}

func shouldForcePathStyle(hostport string) bool {
	host := strings.TrimSpace(hostport)
	if host == "" {
		return false
	}
	hostOnly := host
	if strings.HasPrefix(hostOnly, "[") {
		if h, _, err := net.SplitHostPort(hostOnly); err == nil {
			hostOnly = strings.Trim(h, "[]")
		}
	} else if h, _, err := net.SplitHostPort(hostOnly); err == nil {
		hostOnly = h
	}
	if net.ParseIP(hostOnly) != nil {
		return true
	}
	if strings.Contains(hostport, ":") {
		return true
	}
	return false
}
