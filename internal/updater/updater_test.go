package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManifestSignatureAndDownloadHash(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("PhotoSheet update package")
	sum := sha256.Sum256(payload)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write(payload)
	}))
	defer server.Close()
	manifest := Manifest{Version: "0.1.1", Channel: "stable", URL: server.URL + "/photosheet", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(payload)), PublishedAt: time.Unix(0, 0).UTC()}
	canonical, err := manifest.payload()
	if err != nil {
		t.Fatal(err)
	}
	manifest.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical))
	if err := VerifyManifest(manifest, publicKey); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "update.bin")
	if err := DownloadAndVerify(context.Background(), server.Client(), manifest, destination); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("downloaded package differs: %q", got)
	}
}
