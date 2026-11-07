package model

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func checkoutTreeHash(t *testing.T, root string) []byte {
	t.Helper()
	h := sha256.New()
	e := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		h.Write([]byte(rel))
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		h.Write(b)
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	return h.Sum(nil)
}
func TestCheckoutPlanModelReadOnlyOriginalCodecAndNoSideEffects(t *testing.T) {
	c, file, p, id := snapshotModelFixture(t)
	beforeConfig, _ := os.ReadFile(file)
	beforeSource := checkoutTreeHash(t, p.Data)
	beforeRepo := checkoutTreeHash(t, p.Repo)
	c.persistenceReadOnly = true
	plan, e := PrepareRepoCheckoutContext(context.Background(), id)
	if e != nil || plan.ID != id || len(plan.Files) != 1 || plan.Files[0].Path != "source.docj" {
		t.Fatal(plan, e)
	}
	afterConfig, _ := os.ReadFile(file)
	if !bytes.Equal(beforeConfig, afterConfig) || !bytes.Equal(beforeSource, checkoutTreeHash(t, p.Data)) || !bytes.Equal(beforeRepo, checkoutTreeHash(t, p.Repo)) {
		t.Fatal("read-only candidate modified configuration/source/cache")
	}
	recordCloudBusinessEvidence(t, "checkout-plan-model.json", map[string]any{"readOnlyOwnerSupported": true, "actualOriginalCodec": true, "configurationSourceAndCacheUnchanged": true})
}
func TestCheckoutPlanLegacyInvalidTargetRejectedBeforePauseAndBackup(t *testing.T) {
	c, file, p, _ := snapshotModelFixture(t)
	c.Sync.Enabled = true
	if e := c.SaveError(); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(file)
	source := checkoutTreeHash(t, p.Data)
	repo := checkoutTreeHash(t, p.Repo)
	checkoutRepo(strings.Repeat("0", 40))
	after, _ := os.ReadFile(file)
	if !c.Sync.Enabled || !bytes.Equal(before, after) || !bytes.Equal(source, checkoutTreeHash(t, p.Data)) || !bytes.Equal(repo, checkoutTreeHash(t, p.Repo)) {
		t.Fatal("invalid target paused sync or wrote backup/source")
	}
	recordCloudBusinessEvidence(t, "checkout-plan-legacy-preflight.json", map[string]any{"missingTargetRefusedBeforeSyncPause": true, "noBackupOrSourceWrite": true, "configurationUnchanged": true})
}
