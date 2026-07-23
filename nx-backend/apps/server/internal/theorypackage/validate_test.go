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
		{"declared domain count differs from cards", func(t *testing.T, root string) {
			mutateJSON(t, root, "manifest.json", func(x map[string]any) { x["counts"].(map[string]any)["domains"] = float64(11) })
		}},
		{"digest contract changed", func(t *testing.T, root string) {
			mutateJSON(t, root, "manifest.json", func(x map[string]any) {
				x["digestContract"].(map[string]any)["canonicalJson"] = "attacker-defined"
			})
		}},
		{"budget follows forged catalog summary", func(t *testing.T, root string) {
			mutateJSON(t, root, "manifest.json", func(x map[string]any) {
				budget := x["budget"].(map[string]any)
				budget["pageEquivalent"] = float64(1436)
				budget["ocrPages"] = float64(258)
			})
			mutateJSON(t, root, "catalog/source-files.json", func(x map[string]any) {
				summary := x["summary"].(map[string]any)
				summary["budgetPageEquivalent"] = float64(1436)
				summary["ocrPageCount"] = float64(258)
			})
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
		{"primary evidence unknown field", func(t *testing.T, root string) {
			mutateJSON(t, root, "cards/personality.attention_focus.json", func(x map[string]any) {
				x["primaryEvidence"].(map[string]any)["quotationText"] = "不得进入数据包"
			})
		}},
		{"primary evidence quote presence mismatch", func(t *testing.T, root string) {
			mutateJSON(t, root, "cards/personality.attention_focus.json", func(x map[string]any) {
				x["primaryEvidence"].(map[string]any)["quotationPresent"] = true
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
		{"canonical work quote aggregate limit", func(t *testing.T, root string) {
			paths, err := filepath.Glob(filepath.Join(root, "cards", "*.json"))
			if err != nil {
				t.Fatal(err)
			}
			first := readJSONObject(t, filepath.Join(root, "cards/personality.attention_focus.json"))["primaryEvidence"].(map[string]any)
			second := readJSONObject(t, filepath.Join(root, "cards/change.timing_and_position.json"))["primaryEvidence"].(map[string]any)
			for index, filename := range paths[:12] {
				reference := first
				if index >= 6 {
					reference = second
				}
				item := readJSONObject(t, filename)
				evidence := map[string]any{}
				for key, value := range reference {
					evidence[key] = value
				}
				evidence["quotationPresent"], evidence["quoteVerified"], evidence["quotationCharacters"] = true, true, float64(80)
				item["primaryEvidence"] = evidence
				writeJSONObject(t, filename, item)
			}
			mutateJSON(t, root, "manifest.json", func(x map[string]any) {
				stats := x["copyright"].(map[string]any)["quoteStatistics"].(map[string]any)
				stats["quoteCount"], stats["totalCharacters"], stats["ocrVerifiedQuoteCount"] = float64(12), float64(960), float64(12)
			})
			mutateJSON(t, root, "catalog/source-files.json", func(x map[string]any) {
				files := x["files"].([]any)
				var workID any
				var secondFileID any
				for _, raw := range files {
					entry := raw.(map[string]any)
					if entry["relativePath"] == "九型理论基础/九型人格·珍藏版.epub" {
						workID = entry["canonicalWorkId"]
					}
					if entry["relativePath"] == "(NEW)周易.pdf" {
						secondFileID = entry["fileId"]
					}
				}
				for _, raw := range files {
					entry := raw.(map[string]any)
					if entry["relativePath"] == "(NEW)周易.pdf" {
						entry["canonicalWorkId"] = workID
					}
				}
				mutateJSON(t, root, "catalog/works.json", func(x map[string]any) {
					for _, raw := range x["works"].([]any) {
						work := raw.(map[string]any)
						if work["workId"] == workID {
							work["sourceFileIds"] = append(work["sourceFileIds"].([]any), secondFileID)
						}
					}
				})
			})
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

func TestValidateRejectsResignedSafetyEvaluationOverlays(t *testing.T) {
	t.Run("runnable result missing bindings", func(t *testing.T) {
		root := copyPackage(t)
		mutateJSON(t, root, "evaluation/safety-cases.json", func(x map[string]any) {
			x["result"].(map[string]any)["status"] = "passed"
		})
		mutateSafetyReport(t, root, "passed", "里程碑 B/C 的检索与会话安全链路尚未接入。")
		resignPackage(t, root)
		expectInvalid(t, root)
	})
	t.Run("not runnable result with passed report", func(t *testing.T) {
		root := copyPackage(t)
		mutateSafetyReport(t, root, "passed", "评测通过。")
		resignPackage(t, root)
		expectInvalid(t, root)
	})
}

func TestValidateRejectsFullyBoundPassedSafetyEvaluationForRoundOne(t *testing.T) {
	root := copyPackage(t)
	manifest := readJSONObject(t, filepath.Join(root, "manifest.json"))
	cases := readJSONObject(t, filepath.Join(root, "evaluation/safety-cases.json"))
	reason := "固定评测集已在指定运行时通过"
	cases["result"] = map[string]any{
		"status":              "passed",
		"reason":              reason,
		"boundContentDigest":  manifest["contentDigest"],
		"safetyCaseSetDigest": cases["caseSetDigest"],
		"runtime":             "test-runtime",
		"runtimeVersion":      "1.0.0",
	}
	writeJSONObject(t, filepath.Join(root, "evaluation/safety-cases.json"), cases)
	report := "# 安全评测报告\n\n- 结果：`passed`\n- 原因：" + reason +
		"\n- 绑定内容：`" + manifest["contentDigest"].(string) + "`\n- 评测集：`" + cases["caseSetDigest"].(string) +
		"`\n- 运行时：`test-runtime/1.0.0`\n"
	if err := os.WriteFile(filepath.Join(root, "reports/safety-evaluation.md"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}
	resignPackage(t, root)
	expectInvalid(t, root)
}

func TestCanonicalJSONMatchesPythonDigest(t *testing.T) {
	value := map[string]any{"html": "<tag>&value>", "slash": "a/b", "unicode": "中文\u2028"}
	const want = "c66c5d8c3558dbe33d14730533739153b96817af04767adbf670d0e5e3e9066c"
	if got := shaCanonical(value); got != want {
		t.Fatalf("shaCanonical() = %s, want Python digest %s", got, want)
	}
}

func TestValidateRejectsFixedSafetyCaseSetOverlays(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"schema", func(x map[string]any) { x["schemaVersion"] = "attacker.v1" }},
		{"missing case", func(x map[string]any) { x["cases"] = x["cases"].([]any)[:9] }},
		{"empty prompt", func(x map[string]any) { x["cases"].([]any)[0].(map[string]any)["prompt"] = "" }},
		{"unknown case field", func(x map[string]any) { x["cases"].([]any)[0].(map[string]any)["payload"] = true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := copyPackage(t)
			mutateJSON(t, root, "evaluation/safety-cases.json", func(x map[string]any) {
				tt.mutate(x)
				x["caseSetDigest"] = shaCanonical(x["cases"])
			})
			resignPackage(t, root)
			expectInvalid(t, root)
		})
	}
}

func TestValidateRejectsRewrittenSafetyCaseSemantics(t *testing.T) {
	root := copyPackage(t)
	mutateJSON(t, root, "evaluation/safety-cases.json", func(x map[string]any) {
		for _, raw := range x["cases"].([]any) {
			item := raw.(map[string]any)
			if item["caseId"] == "self_harm" {
				item["expectedBoundary"] = "只做普通情绪练习"
			}
		}
		x["caseSetDigest"] = shaCanonical(x["cases"])
	})
	resignPackage(t, root)
	expectInvalid(t, root)
}

func TestValidateRejectsReviewTemplateIdentityOverlays(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"review type", func(x map[string]any) { x["reviewType"] = "safety-review" }},
		{"reviewer", func(x map[string]any) { x["reviewerUserId"] = "admin" }},
		{"role", func(x map[string]any) { x["requiredDatabaseRole"] = "admin" }},
		{"schema", func(x map[string]any) { x["schemaVersion"] = "bad" }},
		{"trusted requirement", func(x map[string]any) { x["trustedReviewerRequirement"] = "any_user" }},
		{"offline flag", func(x map[string]any) { x["offlineTemplateOnly"] = false }},
		{"instructions", func(x map[string]any) { x["instructions"] = "可直接发布" }},
		{"authorizing notes", func(x map[string]any) { x["notes"] = "approved for promote" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := copyPackage(t)
			mutateJSON(t, root, "review/source-verification.json", tc.mutate)
			resignPackage(t, root)
			expectInvalid(t, root)
		})
	}
}

func TestJSONParserRejectsLongAndControlledStringsRecursively(t *testing.T) {
	longKey := strings.Repeat("k", 200)
	for _, tc := range []struct{ name, payload string }{
		{"long key", `{"` + longKey + `":"x"}`},
		{"long nested value", `{"outer":{"value":"` + strings.Repeat("x", 5000) + `"}}`},
		{"control value", `{"value":"\u0001"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := object([]byte(tc.payload), "test.json"); err == nil {
				t.Fatal("expected recursive JSON string rejection")
			}
		})
	}
}

func TestValidateRejectsControlledCoverageReportAfterChecksumRewrite(t *testing.T) {
	root := copyPackage(t)
	p := filepath.Join(root, "reports/coverage.md")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, append(b, 0), 0o644); err != nil {
		t.Fatal(err)
	}
	rewriteChecksums(t, root)
	expectInvalid(t, root)
}

func TestValidateRejectsDeclaredPayloadAndNestedUnknownFields(t *testing.T) {
	t.Run("missing fixed coverage report", func(t *testing.T) {
		root := copyPackage(t)
		if err := os.Remove(filepath.Join(root, "reports/coverage.md")); err != nil {
			t.Fatal(err)
		}
		mutateJSON(t, root, "manifest.json", func(x map[string]any) {
			kept := []any{}
			for _, raw := range x["objectFiles"].([]any) {
				if raw != "reports/coverage.md" {
					kept = append(kept, raw)
				}
			}
			x["objectFiles"] = kept
		})
		resignPackage(t, root)
		if _, err := Validate(root); err == nil {
			t.Fatal("expected missing fixed report rejection")
		} else {
			t.Logf("rejected: %v", err)
		}
	})
	t.Run("declared payload", func(t *testing.T) {
		root := copyPackage(t)
		payload := filepath.Join(root, "payload", "fulltext.txt")
		if err := os.MkdirAll(filepath.Dir(payload), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(payload, []byte("full text"), 0o644); err != nil {
			t.Fatal(err)
		}
		mutateJSON(t, root, "manifest.json", func(x map[string]any) { x["objectFiles"] = append(x["objectFiles"].([]any), "payload/fulltext.txt") })
		resignPackage(t, root)
		expectInvalid(t, root)
	})
	for _, tc := range []struct {
		name, file string
		mutate     func(map[string]any)
	}{
		{"preview", "chunk-previews/personality.attention_focus.json", func(x map[string]any) { x["payload"] = true }},
		{"catalog", "catalog/source-files.json", func(x map[string]any) { x["files"].([]any)[0].(map[string]any)["payload"] = true }},
		{"evidence index", "evidence-index.json", func(x map[string]any) { x["payload"] = true }},
		{"manifest source", "manifest.json", func(x map[string]any) { x["sources"].([]any)[0].(map[string]any)["payload"] = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := copyPackage(t)
			mutateJSON(t, root, tc.file, tc.mutate)
			resignPackage(t, root)
			expectInvalid(t, root)
		})
	}
}

func TestValidateRejectsForgedOCRCountDespiteResignedSummary(t *testing.T) {
	root := copyPackage(t)
	mutateJSON(t, root, "catalog/source-files.json", func(x map[string]any) {
		for _, raw := range x["files"].([]any) {
			entry := raw.(map[string]any)
			if entry["extractionRoute"] == "pdf_ocr_selected" && entry["ocrPageCount"].(float64) == 80 {
				entry["ocrPageCount"] = float64(79)
				break
			}
		}
		x["summary"].(map[string]any)["ocrPageCount"] = float64(256)
	})
	mutateJSON(t, root, "manifest.json", func(x map[string]any) { x["budget"].(map[string]any)["ocrPages"] = float64(256) })
	resignPackage(t, root)
	expectInvalid(t, root)
}

func TestValidLocatorRejectsCrossFormatFields(t *testing.T) {
	for _, tc := range []struct {
		name, format string
		locator      map[string]any
	}{
		{"epub page", "epub", map[string]any{"chapter": "c", "paragraph": json.Number("1"), "spineItem": json.Number("1"), "page": json.Number("1")}},
		{"doc page", "doc", map[string]any{"heading": "h", "paragraph": json.Number("1"), "page": json.Number("1")}},
		{"pdf chapter", "pdf", map[string]any{"page": json.Number("1"), "slice": json.Number("1"), "characterStart": json.Number("0"), "characterEnd": json.Number("1"), "chapter": "c"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if validLocator(tc.locator, tc.format) {
				t.Fatal("expected exact locator rejection")
			}
		})
	}
}

func TestValidateRejectsRelationOverlays(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"schema", func(x map[string]any) { x["schemaVersion"] = "bad" }},
		{"dangling", func(x map[string]any) { x["relations"].([]any)[0].(map[string]any)["to"] = "missing.card" }},
		{"type", func(x map[string]any) { x["relations"].([]any)[0].(map[string]any)["type"] = "owns" }},
		{"duplicate", func(x map[string]any) { x["relations"] = append(x["relations"].([]any), x["relations"].([]any)[0]) }},
		{"self loop", func(x map[string]any) { r := x["relations"].([]any)[0].(map[string]any); r["to"] = r["from"] }},
		{"unknown field", func(x map[string]any) { x["relations"].([]any)[0].(map[string]any)["payload"] = true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := copyPackage(t)
			mutateJSON(t, root, "relations.json", tt.mutate)
			resignPackage(t, root)
			expectInvalid(t, root)
		})
	}
}

func TestValidateRejectsNonIntegerAndOverflowCounters(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"string", "40"}, {"null", nil}, {"negative", float64(-1)}, {"fraction", 40.5}, {"overflow", json.Number("9223372036854775808")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := copyPackage(t)
			mutateJSON(t, root, "manifest.json", func(x map[string]any) { x["counts"].(map[string]any)["cards"] = tc.value })
			resignPackage(t, root)
			expectInvalid(t, root)
		})
	}
}

func TestValidateRejectsCardPracticeAndSourceContractOverlays(t *testing.T) {
	for _, tc := range []struct {
		name, file string
		mutate     func(map[string]any)
	}{
		{"card schema", "cards/personality.attention_focus.json", func(x map[string]any) { x["schemaVersion"] = "bad" }},
		{"card authority", "cards/personality.attention_focus.json", func(x map[string]any) { x["authorityLevel"] = float64(5) }},
		{"card provenance", "cards/personality.attention_focus.json", func(x map[string]any) { x["provenance"].(map[string]any)["humanReviewed"] = true }},
		{"card gates", "cards/personality.attention_focus.json", func(x map[string]any) { x["reviewGates"].(map[string]any)["safetyReviewRequired"] = false }},
		{"card safety", "cards/personality.attention_focus.json", func(x map[string]any) { x["safety"].(map[string]any)["payload"] = true }},
		{"practice schema", "practices/practice.call_clues_journal.json", func(x map[string]any) { x["schemaVersion"] = "bad" }},
		{"source attribution", "manifest.json", func(x map[string]any) {
			x["sources"].([]any)[0].(map[string]any)["attribution"].(map[string]any)["isHanTeacherOriginal"] = true
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := copyPackage(t)
			mutateJSON(t, root, tc.file, tc.mutate)
			resignPackage(t, root)
			expectInvalid(t, root)
		})
	}
}

func TestValidateRejectsFulltextHiddenInSynthesisField(t *testing.T) {
	root := copyPackage(t)
	mutateJSON(t, root, "cards/personality.attention_focus.json", func(x map[string]any) { x["summary"] = strings.Repeat("长篇正文", 1000) })
	resignPackage(t, root)
	expectInvalid(t, root)
}

func TestValidateRejectsLocatorExtraFieldsAfterIndexRebind(t *testing.T) {
	root := copyPackage(t)
	card := readJSONObject(t, filepath.Join(root, "cards/personality.attention_focus.json"))
	evidence := card["primaryEvidence"].(map[string]any)
	evidence["locator"].(map[string]any)["page"] = float64(1)
	writeJSONObject(t, filepath.Join(root, "cards/personality.attention_focus.json"), card)
	mutateJSON(t, root, "evidence-index.json", func(x map[string]any) {
		for _, raw := range x["evidence"].([]any) {
			e := raw.(map[string]any)
			if e["sourceId"] == evidence["sourceId"] && e["textSha256"] == evidence["textSha256"] {
				e["locator"] = evidence["locator"]
			}
		}
	})
	resignPackage(t, root)
	expectInvalid(t, root)
}

func mutateSafetyReport(t *testing.T, root, status, reason string) {
	t.Helper()
	report := "# 安全评测报告\n\n- 结果：`" + status + "`\n- 原因：" + reason + "\n"
	if err := os.WriteFile(filepath.Join(root, "reports/safety-evaluation.md"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
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
