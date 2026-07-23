package theorypackage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestValidateAcceptsRound001(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "..", "data", "theory", "xinzhili", "round-001")
	report, err := Validate(root)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if report.PackageID != "xinzhili-round-001" {
		t.Fatalf("PackageID = %q, want xinzhili-round-001", report.PackageID)
	}
}

func copyPackage(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "..", "..", "..", "data", "theory", "xinzhili", "round-001")
	dst := filepath.Join(t.TempDir(), "round-001")
	err := filepath.Walk(src, func(name string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, name)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func readJSONObject(t *testing.T, filename string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(b, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func writeJSONObject(t *testing.T, filename string, value map[string]any) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(filename, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func rewriteChecksums(t *testing.T, root string) {
	t.Helper()
	var lines []string
	err := filepath.Walk(root, func(filename string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || info.Name() == "checksums.sha256" {
			return nil
		}
		payload, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(payload)
		rel, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		lines = append(lines, hex.EncodeToString(digest[:])+"  "+filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	if err := os.WriteFile(filepath.Join(root, "checksums.sha256"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// resignPackage simulates an attacker who can rewrite all package metadata.
// Semantic validation must still reject an internally consistent bad package.
func resignPackage(t *testing.T, root string) {
	t.Helper()
	files, err := collectFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := object(files["manifest.json"], "manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	contentDigest, err := digestContent(files, manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifest["contentDigest"] = contentDigest
	writeJSONObject(t, filepath.Join(root, "manifest.json"), manifest)
	for _, rel := range []string{"review/source-verification.json", "review/theory-review.json", "review/safety-review.json"} {
		review := readJSONObject(t, filepath.Join(root, filepath.FromSlash(rel)))
		review["contentDigest"] = contentDigest
		writeJSONObject(t, filepath.Join(root, filepath.FromSlash(rel)), review)
	}
	files, err = collectFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err = object(files["manifest.json"], "manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	packageDigest, err := digestPackage(files, manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifest["packageDigest"] = packageDigest
	writeJSONObject(t, filepath.Join(root, "manifest.json"), manifest)
	rewriteChecksums(t, root)
}

func mutateJSON(t *testing.T, root, rel string, mutate func(map[string]any)) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(rel))
	value := readJSONObject(t, filename)
	mutate(value)
	writeJSONObject(t, filename, value)
}

func expectInvalid(t *testing.T, root string) {
	t.Helper()
	if _, err := Validate(root); err == nil {
		t.Fatal("expected invalid package rejection")
	}
}

func TestValidateRejectsTamperingAndPackageEscape(t *testing.T) {
	t.Run("extra file", func(t *testing.T) {
		root := copyPackage(t)
		if err := os.WriteFile(filepath.Join(root, "unexpected.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Validate(root); err == nil {
			t.Fatal("expected extra file rejection")
		}
	})
	t.Run("symlink", func(t *testing.T) {
		root := copyPackage(t)
		if err := os.Symlink("manifest.json", filepath.Join(root, "link.json")); err != nil {
			t.Fatal(err)
		}
		if _, err := Validate(root); err == nil {
			t.Fatal("expected symlink rejection")
		}
	})
	t.Run("digest", func(t *testing.T) {
		root := copyPackage(t)
		p := filepath.Join(root, "cards/personality.attention_focus.json")
		b, _ := os.ReadFile(p)
		var x map[string]any
		_ = json.Unmarshal(b, &x)
		x["summary"] = strings.Repeat("改", 3)
		b, _ = json.Marshal(x)
		_ = os.WriteFile(p, b, 0o644)
		if _, err := Validate(root); err == nil {
			t.Fatal("expected digest rejection")
		}
	})
	t.Run("unknown field", func(t *testing.T) {
		root := copyPackage(t)
		p := filepath.Join(root, "cards/personality.attention_focus.json")
		b, _ := os.ReadFile(p)
		var x map[string]any
		_ = json.Unmarshal(b, &x)
		x["unexpected"] = true
		b, _ = json.Marshal(x)
		_ = os.WriteFile(p, b, 0o644)
		if _, err := Validate(root); err == nil {
			t.Fatal("expected unknown field rejection")
		}
	})
}

func TestValidateRejectsResignedSemanticViolations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{"declared card count differs from files", func(t *testing.T, root string) {
			mutateJSON(t, root, "manifest.json", func(x map[string]any) { x["counts"].(map[string]any)["cards"] = float64(41) })
		}},
		{"page budget exceeds hard limit", func(t *testing.T, root string) {
			mutateJSON(t, root, "manifest.json", func(x map[string]any) {
				budget := x["budget"].(map[string]any)
				budget["pageEquivalent"] = float64(2001)
				budget["limits"].(map[string]any)["maxBudgetPageEquivalent"] = float64(3000)
			})
		}},
		{"ocr budget exceeds hard limit", func(t *testing.T, root string) {
			mutateJSON(t, root, "manifest.json", func(x map[string]any) {
				budget := x["budget"].(map[string]any)
				budget["ocrPages"] = float64(301)
				budget["limits"].(map[string]any)["maxOcrPageCount"] = float64(400)
			})
		}},
		{"orphan source", func(t *testing.T, root string) {
			mutateJSON(t, root, "cards/personality.attention_focus.json", func(x map[string]any) {
				x["primaryEvidence"].(map[string]any)["sourceId"] = "source.missing"
			})
		}},
		{"evidence text sha", func(t *testing.T, root string) {
			mutateJSON(t, root, "cards/personality.attention_focus.json", func(x map[string]any) {
				x["primaryEvidence"].(map[string]any)["textSha256"] = strings.Repeat("a", 64)
			})
		}},
		{"evidence locator", func(t *testing.T, root string) {
			mutateJSON(t, root, "cards/personality.attention_focus.json", func(x map[string]any) {
				x["primaryEvidence"].(map[string]any)["locator"].(map[string]any)["paragraph"] = float64(18)
			})
		}},
		{"evidence source sha", func(t *testing.T, root string) {
			mutateJSON(t, root, "evidence-index.json", func(x map[string]any) {
				x["evidence"].([]any)[0].(map[string]any)["sourceSha256"] = strings.Repeat("a", 64)
			})
		}},
		{"source sha differs from catalog", func(t *testing.T, root string) {
			bad := strings.Repeat("a", 64)
			mutateJSON(t, root, "manifest.json", func(x map[string]any) { x["sources"].([]any)[1].(map[string]any)["sourceSha256"] = bad })
			mutateJSON(t, root, "evidence-index.json", func(x map[string]any) {
				for _, raw := range x["evidence"].([]any) {
					evidence := raw.(map[string]any)
					if evidence["sourceId"] == "source.xinzhili.02" {
						evidence["sourceSha256"] = bad
					}
				}
			})
		}},
		{"single quote limit", func(t *testing.T, root string) {
			mutateJSON(t, root, "cards/personality.attention_focus.json", func(x map[string]any) {
				e := x["primaryEvidence"].(map[string]any)
				e["quotationPresent"], e["quoteVerified"], e["quotationCharacters"] = true, true, float64(81)
			})
		}},
		{"copyright limit declaration changed", func(t *testing.T, root string) {
			mutateJSON(t, root, "manifest.json", func(x map[string]any) {
				x["copyright"].(map[string]any)["limits"].(map[string]any)["maxCharactersPerCard"] = float64(1000)
			})
		}},
		{"work quote aggregate limit", func(t *testing.T, root string) {
			paths, err := filepath.Glob(filepath.Join(root, "cards", "*.json"))
			if err != nil {
				t.Fatal(err)
			}
			reference := readJSONObject(t, filepath.Join(root, "cards/personality.attention_focus.json"))["primaryEvidence"].(map[string]any)
			for _, filename := range paths[:11] {
				item := readJSONObject(t, filename)
				evidence := map[string]any{}
				for key, value := range reference {
					evidence[key] = value
				}
				evidence["quotationPresent"], evidence["quoteVerified"], evidence["quotationCharacters"] = true, true, float64(80)
				item["primaryEvidence"] = evidence
				writeJSONObject(t, filename, item)
			}
		}},
		{"metadata only quote", func(t *testing.T, root string) {
			mutateJSON(t, root, "manifest.json", func(x map[string]any) { x["sources"].([]any)[12].(map[string]any)["copyrightMode"] = "metadata_only" })
			mutateJSON(t, root, "cards/personality.attention_focus.json", func(x map[string]any) {
				e := x["primaryEvidence"].(map[string]any)
				e["quotationPresent"], e["quoteVerified"], e["quotationCharacters"] = true, true, float64(1)
			})
		}},
		{"unverified ocr quote", func(t *testing.T, root string) {
			mutateJSON(t, root, "practices/practice.call_clues_journal.json", func(x map[string]any) {
				e := x["primaryEvidence"].(map[string]any)
				e["quotationPresent"], e["quoteVerified"], e["quotationCharacters"] = true, true, float64(1)
			})
		}},
		{"practice safety assertion false", func(t *testing.T, root string) {
			mutateJSON(t, root, "practices/practice.call_clues_journal.json", func(x map[string]any) {
				x["safety"].(map[string]any)["participantMayStopAnyTime"] = false
			})
		}},
		{"practice stop conditions missing", func(t *testing.T, root string) {
			mutateJSON(t, root, "practices/practice.call_clues_journal.json", func(x map[string]any) { delete(x, "stopConditions") })
		}},
		{"chunk content hash", func(t *testing.T, root string) {
			mutateJSON(t, root, "chunk-previews/personality.attention_focus.json", func(x map[string]any) { x["contentHash"] = strings.Repeat("a", 64) })
		}},
		{"schema contract loosened", func(t *testing.T, root string) {
			mutateJSON(t, root, "schema/theory-package-v1.schema.json", func(x map[string]any) { x["additionalProperties"] = false })
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := copyPackage(t)
			tt.mutate(t, root)
			resignPackage(t, root)
			expectInvalid(t, root)
		})
	}
}

func TestValidateRejectsUnsafeAndIncompleteObjectFileLists(t *testing.T) {
	t.Run("path traversal", func(t *testing.T) {
		root := copyPackage(t)
		mutateJSON(t, root, "manifest.json", func(x map[string]any) {
			x["objectFiles"] = append(x["objectFiles"].([]any), "../escape.json")
		})
		expectInvalid(t, root)
	})
	t.Run("checksums omitted from special object list", func(t *testing.T) {
		root := copyPackage(t)
		mutateJSON(t, root, "manifest.json", func(x map[string]any) {
			var kept []any
			for _, value := range x["objectFiles"].([]any) {
				if value != "checksums.sha256" {
					kept = append(kept, value)
				}
			}
			x["objectFiles"] = kept
		})
		resignPackage(t, root)
		expectInvalid(t, root)
	})
	t.Run("checksums lists itself", func(t *testing.T) {
		root := copyPackage(t)
		p := filepath.Join(root, "checksums.sha256")
		b, _ := os.ReadFile(p)
		line := strings.Repeat("0", 64) + "  checksums.sha256\n"
		_ = os.WriteFile(p, append(b, []byte(line)...), 0o644)
		expectInvalid(t, root)
	})
}

func TestValidateRejectsReviewAndDigestTamperingWithValidChecksums(t *testing.T) {
	t.Run("review content digest", func(t *testing.T) {
		root := copyPackage(t)
		mutateJSON(t, root, "review/source-verification.json", func(x map[string]any) { x["contentDigest"] = strings.Repeat("a", 64) })
		rewriteChecksums(t, root)
		expectInvalid(t, root)
	})
	t.Run("content digest", func(t *testing.T) {
		root := copyPackage(t)
		bad := strings.Repeat("a", 64)
		mutateJSON(t, root, "manifest.json", func(x map[string]any) { x["contentDigest"] = bad })
		for _, rel := range []string{"review/source-verification.json", "review/theory-review.json", "review/safety-review.json"} {
			mutateJSON(t, root, rel, func(x map[string]any) { x["contentDigest"] = bad })
		}
		files, _ := collectFiles(root)
		manifest, _ := object(files["manifest.json"], "manifest.json")
		packageDigest, _ := digestPackage(files, manifest)
		mutateJSON(t, root, "manifest.json", func(x map[string]any) { x["packageDigest"] = packageDigest })
		rewriteChecksums(t, root)
		expectInvalid(t, root)
	})
	t.Run("package digest", func(t *testing.T) {
		root := copyPackage(t)
		mutateJSON(t, root, "manifest.json", func(x map[string]any) { x["packageDigest"] = strings.Repeat("a", 64) })
		rewriteChecksums(t, root)
		expectInvalid(t, root)
	})
	t.Run("checksum record", func(t *testing.T) {
		root := copyPackage(t)
		p := filepath.Join(root, "checksums.sha256")
		b, _ := os.ReadFile(p)
		b[0] = 'a'
		_ = os.WriteFile(p, b, 0o644)
		expectInvalid(t, root)
	})
}

func TestValidateRejectsMutationOfEveryPackagedFile(t *testing.T) {
	root := copyPackage(t)
	var files []string
	err := filepath.Walk(root, func(filename string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			files = append(files, filename)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, filename := range files {
		rel, _ := filepath.Rel(root, filename)
		t.Run(filepath.ToSlash(rel), func(t *testing.T) {
			original, err := os.ReadFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filename, append(append([]byte{}, original...), 'x'), 0o644); err != nil {
				t.Fatal(err)
			}
			expectInvalid(t, root)
			if err := os.WriteFile(filename, original, 0o644); err != nil {
				t.Fatal(err)
			}
		})
	}
}
