package theorypackage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

var errInvalidPackage = errors.New("invalid theory package")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{errInvalidPackage}, args...)...)
}

// Validate checks a package only from its own directory. It never follows a
// symlink and never requires extraction work files or absolute source paths.
func Validate(root string) (Report, error) {
	files, err := collectFiles(root)
	if err != nil {
		return Report{}, err
	}
	need := []string{"manifest.json", "checksums.sha256", "schema/theory-package-v1.schema.json", "evidence-index.json", "relations.json", "evaluation/safety-cases.json", "catalog/works.json", "catalog/source-files.json", "review/source-verification.json", "review/theory-review.json", "review/safety-review.json"}
	for _, name := range need {
		if _, ok := files[name]; !ok {
			return Report{}, invalid("missing required file %q", name)
		}
	}
	manifest, err := object(files["manifest.json"], "manifest.json")
	if err != nil {
		return Report{}, err
	}
	if err := exactKeys(manifest, set("activationAllowed", "budget", "contentDigest", "copyright", "counts", "digestContract", "humanReviewStatus", "objectFiles", "packageDigest", "packageId", "releaseGates", "roundId", "schemaVersion", "sources", "status"), "manifest"); err != nil {
		return Report{}, err
	}
	if str(manifest, "schemaVersion") != "xinzhili.theory-package.v1" || str(manifest, "status") != "draft" || manifest["activationAllowed"] != false || str(manifest, "humanReviewStatus") != "pending" {
		return Report{}, invalid("manifest state is not a review-only draft")
	}
	if str(manifest, "packageId") == "" || str(manifest, "contentDigest") == "" || str(manifest, "packageDigest") == "" {
		return Report{}, invalid("manifest digest or package id missing")
	}
	if err := validateFileSet(files, manifest); err != nil {
		return Report{}, err
	}
	if err := validateBudget(files, manifest); err != nil {
		return Report{}, err
	}
	if err := validateSchema(files["schema/theory-package-v1.schema.json"]); err != nil {
		return Report{}, err
	}
	index, err := object(files["evidence-index.json"], "evidence-index.json")
	if err != nil {
		return Report{}, err
	}
	if err := validateContent(files, manifest, index); err != nil {
		return Report{}, err
	}
	if err := validateReviews(files, manifest); err != nil {
		return Report{}, err
	}
	if err := validateDigests(files, manifest); err != nil {
		return Report{}, err
	}
	if err := validateChecksums(files); err != nil {
		return Report{}, err
	}
	return Report{PackageID: str(manifest, "packageId"), ContentDigest: str(manifest, "contentDigest"), PackageDigest: str(manifest, "packageDigest"), FileCount: len(files)}, nil
}

func collectFiles(root string) (map[string][]byte, error) {
	base, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(base)
	if err != nil {
		return nil, fmt.Errorf("open package: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, invalid("package root must be a real directory")
	}
	files := map[string][]byte{}
	total := int64(0)
	err = filepath.WalkDir(base, func(name string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == base {
			return nil
		}
		rel, err := filepath.Rel(base, name)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || strings.HasPrefix(rel, "../") || path.IsAbs(rel) || path.Clean(rel) != rel {
			return invalid("unsafe path %q", rel)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return invalid("symlink is forbidden: %q", rel)
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return invalid("non-regular file %q", rel)
		}
		if len(files) >= maxFiles {
			return invalid("too many files")
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxFileBytes {
			return invalid("file too large %q", rel)
		}
		total += info.Size()
		if total > maxPackageBytes {
			return invalid("package too large")
		}
		b, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		files[rel] = b
		return nil
	})
	return files, err
}

func validateFileSet(files map[string][]byte, m map[string]any) error {
	raw, ok := m["objectFiles"].([]any)
	if !ok {
		return invalid("objectFiles must be an array")
	}
	want := set("manifest.json", "checksums.sha256")
	checksumDeclarations := 0
	for _, v := range raw {
		p, ok := v.(string)
		if !ok || p == "" || path.Clean(p) != p || strings.HasPrefix(p, "../") || path.IsAbs(p) {
			return invalid("bad object file path")
		}
		if p == "checksums.sha256" {
			checksumDeclarations++
		}
		if _, seen := want[p]; seen {
			if p != "checksums.sha256" || checksumDeclarations > 1 {
				return invalid("duplicate object file %q", p)
			}
			continue
		}
		want[p] = struct{}{}
	}
	if checksumDeclarations != 1 {
		return invalid("checksums.sha256 must appear exactly once in objectFiles")
	}
	if len(want) != len(files) {
		return invalid("package has undeclared or missing files")
	}
	for p := range files {
		if _, ok := want[p]; !ok {
			return invalid("undeclared file %q", p)
		}
	}
	return nil
}
func validateBudget(files map[string][]byte, m map[string]any) error {
	counts := mapv(m, "counts")
	budget := mapv(m, "budget")
	limits := mapv(budget, "limits")
	actualCards, actualPractices := 0, 0
	for name := range files {
		if strings.HasPrefix(name, "cards/") && strings.HasSuffix(name, ".json") {
			actualCards++
		}
		if strings.HasPrefix(name, "practices/") && strings.HasSuffix(name, ".json") {
			actualPractices++
		}
	}
	actualSources := len(arr(m, "sources"))
	if num(counts, "sources") != actualSources || num(counts, "cards") != actualCards || num(counts, "practices") != actualPractices {
		return invalid("declared counts do not match package objects")
	}
	if actualSources > 24 || actualCards < 40 || actualPractices < 12 || num(counts, "domains") < 10 {
		return invalid("counts outside round limits")
	}
	if num(limits, "maxBudgetPageEquivalent") != 2000 || num(limits, "maxOcrPageCount") != 300 || num(limits, "maxSelectedFiles") != 24 {
		return invalid("round limits differ from the fixed contract")
	}
	if num(budget, "pageEquivalent") < 0 || num(budget, "pageEquivalent") > 2000 || num(budget, "ocrPages") < 0 || num(budget, "ocrPages") > 300 || actualSources > 24 {
		return invalid("budget exceeded")
	}
	catalog, err := object(files["catalog/source-files.json"], "catalog/source-files.json")
	if err != nil {
		return err
	}
	summary := mapv(catalog, "summary")
	if num(summary, "budgetPageEquivalent") != num(budget, "pageEquivalent") || num(summary, "ocrPageCount") != num(budget, "ocrPages") || num(summary, "selectedCount") != actualSources {
		return invalid("budget does not match source catalog")
	}
	return nil
}
func validateSchema(b []byte) error {
	o, err := object(b, "schema/theory-package-v1.schema.json")
	if err != nil {
		return err
	}
	expected := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "https://xinzhili.local/schema/theory-package-v1.schema.json",
		"title":                "Xinzhili Theory Package v1",
		"type":                 "object",
		"required":             []any{"schemaVersion", "packageId", "contentDigest", "packageDigest"},
		"additionalProperties": true,
		"properties": map[string]any{
			"schemaVersion": map[string]any{"const": "xinzhili.theory-package.v1"},
			"packageId":     map[string]any{"type": "string"},
			"contentDigest": map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$"},
			"packageDigest": map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$"},
		},
	}
	if !sameJSON(o, expected) {
		return invalid("schema differs from the fixed v1 contract")
	}
	return nil
}

func validateContent(files map[string][]byte, m, index map[string]any) error {
	copyright := mapv(m, "copyright")
	limits := mapv(copyright, "limits")
	if str(copyright, "mode") != "metadata_and_original_synthesis_only" || copyright["metadataOnlyQuotesAllowed"] != false || copyright["ocrUnverifiedQuotesPublishable"] != false ||
		num(limits, "maxCharactersPerQuote") != 80 || num(limits, "maxCharactersPerCard") != 160 || num(limits, "maxCharactersPerWork") != 800 {
		return invalid("copyright policy differs from the fixed contract")
	}
	catalog, err := object(files["catalog/source-files.json"], "catalog/source-files.json")
	if err != nil {
		return err
	}
	catalogSources := map[string]map[string]any{}
	for _, raw := range arr(catalog, "files") {
		entry, ok := raw.(map[string]any)
		if !ok {
			return invalid("source catalog entry invalid")
		}
		relativePath := str(entry, "relativePath")
		if relativePath == "" || catalogSources[relativePath] != nil {
			return invalid("source catalog path invalid or duplicated")
		}
		catalogSources[relativePath] = entry
	}
	sources := map[string]map[string]any{}
	for _, v := range arr(m, "sources") {
		s, ok := v.(map[string]any)
		if !ok {
			return invalid("source must be object")
		}
		id := str(s, "sourceId")
		if id == "" || sources[id] != nil {
			return invalid("invalid source id")
		}
		relativePath := str(s, "relativePath")
		catalogSource := catalogSources[relativePath]
		if relativePath == "" || path.IsAbs(relativePath) || path.Clean(relativePath) != relativePath || strings.HasPrefix(relativePath, "../") || strings.Contains(relativePath, "\\") ||
			!sha256hex(str(s, "sourceSha256")) || catalogSource == nil || str(catalogSource, "sha256") != str(s, "sourceSha256") || str(catalogSource, "extractionRoute") != str(s, "extractionRoute") ||
			str(s, "humanReviewStatus") != "pending" || (str(s, "copyrightMode") != "metadata_only" && str(s, "copyrightMode") != "metadata_and_original_synthesis_only") {
			return invalid("source %q does not match source catalog or draft policy", id)
		}
		sources[id] = s
	}
	units := map[string]map[string]any{}
	if str(index, "schemaVersion") != "xinzhili.evidence-index.v1" {
		return invalid("evidence index schema version invalid")
	}
	for _, v := range arr(index, "evidence") {
		e, ok := v.(map[string]any)
		if !ok {
			return invalid("evidence index item invalid")
		}
		if err := exactKeys(e, set("encoding", "locator", "ocrVerified", "sourceId", "sourceSha256", "textSha256"), "evidence index item"); err != nil {
			return err
		}
		sid, textSHA := str(e, "sourceId"), str(e, "textSha256")
		if sources[sid] == nil || !sha256hex(textSHA) || str(e, "sourceSha256") != str(sources[sid], "sourceSha256") || e["locator"] == nil {
			return invalid("invalid evidence index")
		}
		if _, ok := e["ocrVerified"].(bool); !ok {
			return invalid("invalid evidence OCR verification state")
		}
		unitKey := sid + ":" + textSHA
		if units[unitKey] != nil {
			return invalid("duplicate evidence index item")
		}
		units[unitKey] = e
	}
	workQuotes := map[string]int{}
	cardQuotes := map[string]int{}
	keys := set()
	previews := map[string]bool{}
	quoteCount, quoteCharacters, ocrVerifiedQuoteCount := 0, 0, 0
	for p, b := range files {
		if strings.HasPrefix(p, "cards/") || strings.HasPrefix(p, "practices/") {
			item, err := object(b, p)
			if err != nil {
				return err
			}
			isPractice := strings.HasPrefix(p, "practices/")
			allowed := set("authorityLevel", "canonicalKey", "definition", "domain", "epistemicStatus", "evidenceLevel", "primaryEvidence", "provenance", "reviewGates", "safety", "schemaVersion", "status", "summary", "title")
			if isPractice {
				allowed = set("canonicalKey", "primaryEvidence", "professionalEscalationConditions", "provenance", "purpose", "reviewGates", "safety", "schemaVersion", "status", "steps", "stopConditions", "title")
			}
			if err := exactKeys(item, allowed, p); err != nil {
				return err
			}
			if err := validateItem(item, isPractice, sources, units, workQuotes, cardQuotes); err != nil {
				return err
			}
			evidence := mapv(item, "primaryEvidence")
			quoted := num(evidence, "quotationCharacters")
			if quoted > 0 {
				quoteCount++
				quoteCharacters += quoted
				unit := units[str(evidence, "sourceId")+":"+str(evidence, "textSha256")]
				if unit["ocrVerified"] == true {
					ocrVerifiedQuoteCount++
				}
			}
			key := str(item, "canonicalKey")
			if _, ok := keys[key]; ok {
				return invalid("duplicate canonical key")
			}
			keys[key] = struct{}{}
		}
	}
	for p, b := range files {
		if strings.HasPrefix(p, "chunk-previews/") {
			x, err := object(b, p)
			if err != nil {
				return err
			}
			key := str(x, "canonicalKey")
			if _, ok := keys[key]; !ok {
				return invalid("preview without item")
			}
			if str(x, "contentType") != "original_synthesis" || str(x, "status") != "draft" || x["formalTheoryChunk"] != false {
				return invalid("preview %q is not draft original synthesis", p)
			}
			text := str(x, "text")
			if len([]rune(text)) > 3000 || strings.Contains(text, "\f") || sha([]byte(text)) != str(x, "contentHash") {
				return invalid("preview hash or size invalid")
			}
			previews[key] = true
		}
	}
	for key := range keys {
		if !previews[key] {
			return invalid("item %q has no preview", key)
		}
	}
	statistics := mapv(copyright, "quoteStatistics")
	if num(statistics, "quoteCount") != quoteCount || num(statistics, "totalCharacters") != quoteCharacters || num(statistics, "ocrVerifiedQuoteCount") != ocrVerifiedQuoteCount {
		return invalid("copyright quote statistics do not match package content")
	}
	return nil
}
func validateItem(x map[string]any, practice bool, sources, units map[string]map[string]any, works, cards map[string]int) error {
	key := str(x, "canonicalKey")
	if key == "" || str(x, "status") != "draft" || str(x, "title") == "" {
		return invalid("invalid item")
	}
	e := mapv(x, "primaryEvidence")
	sid := str(e, "sourceId")
	shaText := str(e, "textSha256")
	unit := units[sid+":"+shaText]
	if sources[sid] == nil || unit == nil || !validLocator(e["locator"], str(sources[sid], "format")) {
		return invalid("invalid source evidence for %q", key)
	}
	if str(e, "extractionRoute") != str(sources[sid], "extractionRoute") {
		return invalid("evidence extraction route mismatch for %q", key)
	}
	if !sameJSON(e["locator"], unit["locator"]) {
		return invalid("evidence locator is not bound to indexed text for %q", key)
	}
	q := num(e, "quotationCharacters")
	if q < 0 || q > 80 {
		return invalid("single quote limit")
	}
	if e["quotationPresent"] == false && q != 0 {
		return invalid("quote metadata mismatch")
	}
	if q > 0 && e["quoteVerified"] != true {
		return invalid("unverified quote")
	}
	if q > 0 && unit["ocrVerified"] != true {
		return invalid("unverified OCR quote")
	}
	if str(sources[sid], "copyrightMode") == "metadata_only" && q > 0 {
		return invalid("metadata-only source has quote")
	}
	works[sid] += q
	cards[key] += q
	if works[sid] > 800 || cards[key] > 160 {
		return invalid("quote aggregate limit")
	}
	if practice {
		for _, f := range []string{"steps", "stopConditions", "professionalEscalationConditions", "safety"} {
			if x[f] == nil {
				return invalid("practice %q missing %s", key, f)
			}
		}
		if len(arr(x, "steps")) < 3 || len(arr(x, "stopConditions")) == 0 || len(arr(x, "professionalEscalationConditions")) == 0 {
			return invalid("practice %q safety fields empty", key)
		}
		safety := mapv(x, "safety")
		for _, field := range []string{"informedConsentRequired", "participantMayStopAnyTime", "notTreatment", "noTraumaDetailElicitation"} {
			if safety[field] != true {
				return invalid("practice %q safety assertion %s must be true", key, field)
			}
		}
	}
	return nil
}
func validLocator(v any, format string) bool {
	x, ok := v.(map[string]any)
	if !ok {
		return false
	}
	switch format {
	case "pdf":
		return num(x, "page") > 0
	case "epub":
		return num(x, "spineItem") > 0 && str(x, "chapter") != "" && num(x, "paragraph") > 0
	case "doc", "docx":
		return str(x, "heading") != "" && num(x, "paragraph") > 0
	default:
		return false
	}
}
func validateReviews(files map[string][]byte, m map[string]any) error {
	for _, p := range []string{"review/source-verification.json", "review/theory-review.json", "review/safety-review.json"} {
		x, err := object(files[p], p)
		if err != nil {
			return err
		}
		if err := exactKeys(x, set("authorizesPromotion", "contentDigest", "instructions", "notes", "offlineTemplateOnly", "requiredDatabaseRole", "reviewType", "reviewerUserId", "status", "schemaVersion", "trustedReviewerRequirement"), p); err != nil {
			return err
		}
		if str(x, "status") != "pending" || str(x, "contentDigest") != str(m, "contentDigest") || x["authorizesPromotion"] != false {
			return invalid("review %q not a bound pending template", p)
		}
	}
	cases, err := object(files["evaluation/safety-cases.json"], "safety cases")
	if err != nil {
		return err
	}
	result := mapv(cases, "result")
	if result["boundContentDigest"] != nil || result["runtime"] != nil || result["runtimeVersion"] != nil {
		if str(result, "boundContentDigest") != str(m, "contentDigest") {
			return invalid("safety result bound to wrong content digest")
		}
	}
	return nil
}

func sameJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
func validateDigests(files map[string][]byte, m map[string]any) error {
	content, err := digestContent(files, m)
	if err != nil {
		return err
	}
	if content != str(m, "contentDigest") {
		return invalid("content digest mismatch")
	}
	pkg, err := digestPackage(files, m)
	if err != nil {
		return err
	}
	if pkg != str(m, "packageDigest") {
		return invalid("package digest mismatch")
	}
	return nil
}
func digestContent(files map[string][]byte, m map[string]any) (string, error) {
	manifest := clone(m)
	delete(manifest, "contentDigest")
	delete(manifest, "packageDigest")
	delete(manifest, "checksums")
	paths := []string{}
	for _, prefix := range []string{"cards/", "practices/", "chunk-previews/"} {
		var group []string
		for p := range files {
			if strings.HasPrefix(p, prefix) {
				group = append(group, p)
			}
		}
		sort.Strings(group)
		paths = append(paths, group...)
	}
	for _, prefix := range []string{"catalog/"} {
		var group []string
		for p := range files {
			if strings.HasPrefix(p, prefix) {
				group = append(group, p)
			}
		}
		sort.Strings(group)
		paths = append(paths, group...)
	}
	paths = append(paths, "relations.json", "evaluation/safety-cases.json", "evidence-index.json", "schema/theory-package-v1.schema.json")
	objects := make([]any, 0, len(paths))
	for _, p := range paths {
		x, err := object(files[p], p)
		if err != nil {
			return "", err
		}
		if p == "evaluation/safety-cases.json" {
			delete(x, "result")
		}
		objects = append(objects, map[string]any{"path": p, "value": x})
	}
	return shaCanonical(map[string]any{"manifest": manifest, "objects": objects, "coverageObjectFiles": m["objectFiles"]}), nil
}
func digestPackage(files map[string][]byte, m map[string]any) (string, error) {
	manifest := clone(m)
	delete(manifest, "packageDigest")
	delete(manifest, "checksums")
	reviews := []any{}
	for _, p := range []string{"review/safety-review.json", "review/source-verification.json", "review/theory-review.json"} {
		x, err := object(files[p], p)
		if err != nil {
			return "", err
		}
		reviews = append(reviews, map[string]any{"path": p, "value": x})
	}
	cases, err := object(files["evaluation/safety-cases.json"], "safety cases")
	if err != nil {
		return "", err
	}
	return shaCanonical(map[string]any{"manifest": manifest, "contentDigest": m["contentDigest"], "reviews": reviews, "safetyEvaluationResult": cases["result"], "safetyCaseSetDigest": cases["caseSetDigest"]}), nil
}
func validateChecksums(files map[string][]byte) error {
	lines := strings.Split(strings.TrimSpace(string(files["checksums.sha256"])), "\n")
	seen := set()
	if len(lines) != len(files)-1 {
		return invalid("checksum count mismatch")
	}
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) != 2 || !sha256hex(parts[0]) || parts[1] == "checksums.sha256" || path.Clean(parts[1]) != parts[1] {
			return invalid("invalid checksum record")
		}
		if _, ok := seen[parts[1]]; ok {
			return invalid("duplicate checksum")
		}
		seen[parts[1]] = struct{}{}
		b, ok := files[parts[1]]
		if !ok || sha(b) != parts[0] {
			return invalid("checksum mismatch for %q", parts[1])
		}
	}
	return nil
}
func object(b []byte, name string) (map[string]any, error) {
	if err := noDuplicateKeys(b); err != nil {
		return nil, invalid("%s: %v", name, err)
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var x map[string]any
	if err := d.Decode(&x); err != nil {
		return nil, invalid("invalid JSON %s", name)
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return nil, invalid("trailing JSON %s", name)
	}
	return x, nil
}
func noDuplicateKeys(b []byte) error {
	d := json.NewDecoder(bytes.NewReader(b))
	var walk func() error
	walk = func() error {
		t, e := d.Token()
		if e != nil {
			return e
		}
		switch x := t.(type) {
		case json.Delim:
			if x == '{' {
				seen := map[string]bool{}
				for d.More() {
					k, e := d.Token()
					if e != nil {
						return e
					}
					ks, ok := k.(string)
					if !ok || seen[ks] {
						return errors.New("duplicate or invalid object key")
					}
					seen[ks] = true
					if e := walk(); e != nil {
						return e
					}
				}
				_, e = d.Token()
				return e
			}
			if x == '[' {
				for d.More() {
					if e := walk(); e != nil {
						return e
					}
				}
				_, e = d.Token()
				return e
			}
		}
		return nil
	}
	if err := walk(); err != nil {
		return err
	}
	if d.More() {
		return errors.New("trailing value")
	}
	return nil
}
func exactKeys(x map[string]any, allowed map[string]struct{}, name string) error {
	for k := range x {
		if _, ok := allowed[k]; !ok {
			return invalid("unknown field %s.%s", name, k)
		}
	}
	return nil
}
func mapv(x map[string]any, k string) map[string]any { v, _ := x[k].(map[string]any); return v }
func arr(x map[string]any, k string) []any           { v, _ := x[k].([]any); return v }
func str(x map[string]any, k string) string          { v, _ := x[k].(string); return v }
func num(x map[string]any, k string) int {
	v, ok := x[k].(json.Number)
	if !ok {
		return 0
	}
	n, _ := v.Int64()
	return int(n)
}
func set(v ...string) map[string]struct{} {
	x := map[string]struct{}{}
	for _, s := range v {
		x[s] = struct{}{}
	}
	return x
}
func clone(x map[string]any) map[string]any {
	b, _ := json.Marshal(x)
	var y map[string]any
	json.Unmarshal(b, &y)
	return y
}
func sha(b []byte) string       { x := sha256.Sum256(b); return hex.EncodeToString(x[:]) }
func sha256hex(s string) bool   { _, e := hex.DecodeString(s); return e == nil && len(s) == 64 }
func shaCanonical(x any) string { b, _ := json.Marshal(x); return sha(b) }
