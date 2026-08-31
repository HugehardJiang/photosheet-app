package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Manifest struct {
	Version     string    `json:"version"`
	Channel     string    `json:"channel"`
	URL         string    `json:"url"`
	SHA256      string    `json:"sha256"`
	Size        int64     `json:"size,omitempty"`
	PublishedAt time.Time `json:"publishedAt"`
	Signature   string    `json:"signature"`
}

type signedManifest struct {
	Version     string    `json:"version"`
	Channel     string    `json:"channel"`
	URL         string    `json:"url"`
	SHA256      string    `json:"sha256"`
	Size        int64     `json:"size,omitempty"`
	PublishedAt time.Time `json:"publishedAt"`
}

func (m Manifest) payload() ([]byte, error) {
	return json.Marshal(signedManifest{
		Version: m.Version, Channel: m.Channel, URL: m.URL, SHA256: strings.ToLower(m.SHA256), Size: m.Size, PublishedAt: m.PublishedAt,
	})
}

func VerifyManifest(m Manifest, publicKey ed25519.PublicKey) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return errors.New("更新公钥长度无效")
	}
	if strings.TrimSpace(m.Version) == "" || strings.TrimSpace(m.URL) == "" {
		return errors.New("更新 manifest 缺少版本或下载地址")
	}
	parsedURL, err := url.Parse(m.URL)
	if err != nil || (parsedURL.Scheme != "https" && parsedURL.Scheme != "http") || parsedURL.Host == "" {
		return errors.New("更新地址必须是有效的 HTTP(S) URL")
	}
	if _, err := parseSHA256(m.SHA256); err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("更新签名无效")
	}
	payload, err := m.payload()
	if err != nil {
		return fmt.Errorf("编码更新 manifest 失败: %w", err)
	}
	if !ed25519.Verify(publicKey, payload, signature) {
		return errors.New("更新 manifest 签名校验失败")
	}
	return nil
}

func FetchManifest(ctx context.Context, client *http.Client, manifestURL string, publicKey ed25519.PublicKey) (Manifest, error) {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return Manifest{}, fmt.Errorf("创建更新请求失败: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return Manifest{}, fmt.Errorf("下载更新 manifest 失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Manifest{}, fmt.Errorf("更新 manifest 返回 HTTP %d", response.StatusCode)
	}
	var manifest Manifest
	decoder := json.NewDecoder(io.LimitReader(response.Body, 8<<20))
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("解析更新 manifest 失败: %w", err)
	}
	if err := VerifyManifest(manifest, publicKey); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func DownloadAndVerify(ctx context.Context, client *http.Client, manifest Manifest, destination string) error {
	if err := VerifyManifestWithoutSignature(manifest); err != nil {
		return err
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Minute}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, manifest.URL, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("下载更新包失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("更新包返回 HTTP %d", response.StatusCode)
	}
	if manifest.Size > 0 && response.ContentLength > 0 && response.ContentLength != manifest.Size {
		return fmt.Errorf("更新包大小不符：收到 %d，manifest 为 %d", response.ContentLength, manifest.Size)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".photosheet-update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpName)
		}
	}()
	hash := sha256.New()
	count, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(response.Body, 16<<40))
	if err != nil {
		return fmt.Errorf("写入更新包失败: %w", err)
	}
	if manifest.Size > 0 && count != manifest.Size {
		return fmt.Errorf("更新包大小不符：收到 %d，manifest 为 %d", count, manifest.Size)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); !strings.EqualFold(got, manifest.SHA256) {
		return fmt.Errorf("更新包 SHA-256 不符：收到 %s，manifest 为 %s", got, manifest.SHA256)
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, destination); err != nil {
		return fmt.Errorf("保存更新包失败: %w", err)
	}
	keep = true
	return nil
}

func VerifyManifestWithoutSignature(m Manifest) error {
	if strings.TrimSpace(m.URL) == "" {
		return errors.New("更新地址为空")
	}
	if _, err := parseSHA256(m.SHA256); err != nil {
		return err
	}
	return nil
}

func ApplyExecutable(current, update, backup string) error {
	if current == "" || update == "" || backup == "" {
		return errors.New("更新路径不能为空")
	}
	current, err := filepath.Abs(current)
	if err != nil {
		return err
	}
	update, err = filepath.Abs(update)
	if err != nil {
		return err
	}
	backup, err = filepath.Abs(backup)
	if err != nil {
		return err
	}
	if err := os.Rename(current, backup); err != nil {
		return fmt.Errorf("备份旧程序失败: %w", err)
	}
	if err := os.Rename(update, current); err != nil {
		if restoreErr := os.Rename(backup, current); restoreErr != nil {
			return fmt.Errorf("替换程序失败: %v；恢复旧程序也失败: %w", err, restoreErr)
		}
		return fmt.Errorf("替换程序失败，旧版本已恢复: %w", err)
	}
	_ = os.Remove(backup)
	return nil
}

func Restart(executable string, args ...string) error {
	if executable == "" {
		return errors.New("重启程序路径为空")
	}
	command := exec.Command(executable, args...)
	if err := command.Start(); err != nil {
		return fmt.Errorf("重启 PhotoSheet 失败: %w", err)
	}
	return nil
}

func parseSHA256(value string) ([]byte, error) {
	decoded, err := hex.DecodeString(strings.TrimSpace(value))
	if err != nil || len(decoded) != sha256.Size {
		return nil, errors.New("manifest 中的 SHA-256 必须是 64 位十六进制字符串")
	}
	return decoded, nil
}
