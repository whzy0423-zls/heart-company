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
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var errInvalidPackage = errors.New("invalid theory package")

const (
	roundOneSafetyCaseSetDigest     = "d646838a32c9a8b77cc8aa04167dd59baa75c9e73d0d55d5e24a27348028a7cc"
	roundOneEvidenceGroundingDigest = "611c16e7b3de03d6f8b6c87e475df8a9a83ef5db6bf79cead27d13c4c24aa1c8"
	reviewInstructions              = "正式审核必须由后台或 CLI 核验数据库用户与角色后写入；构包身份不得充当 reviewer。"
)

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
	if err := validateManifestContract(manifest); err != nil {
		return Report{}, err
	}
	if !sameJSON(manifest["digestContract"], map[string]any{
		"canonicalJson":         "UTF-8 LF; object keys sorted; arrays preserve semantic order; no extra whitespace",
		"contentDigestExcludes": []any{"contentDigest", "packageDigest", "reviews", "checksums.sha256"},
		"packageDigestExcludes": []any{"packageDigest", "checksums.sha256"},
	}) {
		return Report{}, invalid("digest contract differs from fixed v1 contract")
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

func validateManifestContract(m map[string]any) error {
	if str(m, "packageId") != "xinzhili-round-001" || str(m, "roundId") != "round-001" {
		return invalid("manifest package identity invalid")
	}
	counts := mapv(m, "counts")
	if err := exactKeys(counts, set("cards", "domains", "formalTheoryChunks", "practices", "sources"), "manifest counts"); err != nil {
		return err
	}
	for key, want := range map[string]int{"sources": 24, "cards": 40, "practices": 12, "domains": 10, "formalTheoryChunks": 0} {
		got, ok := integer(counts, key)
		if !ok || got != want {
			return invalid("manifest count %s invalid", key)
		}
	}
	budget := mapv(m, "budget")
	if err := exactKeys(budget, set("budgetRuleVersion", "limits", "ocrPages", "pageEquivalent"), "manifest budget"); err != nil {
		return err
	}
	limits := mapv(budget, "limits")
	if err := exactKeys(limits, set("maxBudgetPageEquivalent", "maxOcrPageCount", "maxSelectedFiles"), "manifest budget limits"); err != nil {
		return err
	}
	if str(budget, "budgetRuleVersion") != "xinzhili-page-equivalent-v1" {
		return invalid("budget rule version invalid")
	}
	for key, want := range map[string]int{"maxBudgetPageEquivalent": 2000, "maxOcrPageCount": 300, "maxSelectedFiles": 24} {
		got, ok := integer(limits, key)
		if !ok || got != want {
			return invalid("budget limit %s invalid", key)
		}
	}
	pageEquivalent, pageOK := integer(budget, "pageEquivalent")
	ocrPages, ocrOK := integer(budget, "ocrPages")
	if !pageOK || !ocrOK || pageEquivalent > 2000 || ocrPages > 300 || ocrPages > pageEquivalent {
		return invalid("manifest budget values invalid")
	}
	copyright := mapv(m, "copyright")
	if err := exactKeys(copyright, set("limits", "metadataOnlyQuotesAllowed", "mode", "ocrUnverifiedQuotesPublishable", "quoteStatistics"), "manifest copyright"); err != nil {
		return err
	}
	copyrightLimits := mapv(copyright, "limits")
	if err := exactKeys(copyrightLimits, set("maxCharactersPerCard", "maxCharactersPerQuote", "maxCharactersPerWork"), "copyright limits"); err != nil {
		return err
	}
	for key, want := range map[string]int{"maxCharactersPerQuote": 80, "maxCharactersPerCard": 160, "maxCharactersPerWork": 800} {
		got, ok := integer(copyrightLimits, key)
		if !ok || got != want {
			return invalid("copyright limit %s invalid", key)
		}
	}
	statistics := mapv(copyright, "quoteStatistics")
	if err := exactKeys(statistics, set("ocrVerifiedQuoteCount", "quoteCount", "totalCharacters"), "quote statistics"); err != nil {
		return err
	}
	for _, key := range []string{"ocrVerifiedQuoteCount", "quoteCount", "totalCharacters"} {
		if _, ok := integer(statistics, key); !ok {
			return invalid("quote statistic %s invalid", key)
		}
	}
	releaseGates := mapv(m, "releaseGates")
	if err := exactKeys(releaseGates, set("builderMayApprove", "courseAttributionReviewRequired", "milestoneBCRequiredForActivation", "threeDatabaseReviewsRequired"), "release gates"); err != nil {
		return err
	}
	if releaseGates["builderMayApprove"] != false || releaseGates["courseAttributionReviewRequired"] != true || releaseGates["milestoneBCRequiredForActivation"] != true || releaseGates["threeDatabaseReviewsRequired"] != true {
		return invalid("release gate contract invalid")
	}
	return nil
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
		if (strings.HasPrefix(rel, "cards/") || strings.HasPrefix(rel, "practices/") || strings.HasPrefix(rel, "chunk-previews/") || strings.HasPrefix(rel, "reports/") || rel == "README.md") && info.Size() > 64<<10 {
			return invalid("content object too large %q", rel)
		}
		total += info.Size()
		if total > maxPackageBytes {
			return invalid("package too large")
		}
		b, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		if strings.HasPrefix(rel, "reports/") && !validMarkdown(b) {
			return invalid("report contains invalid or controlled text %q", rel)
		}
		if isDeliveryDocument(rel) && !validDeliveryDocument(b) {
			return invalid("delivery document violates portable documentation contract %q", rel)
		}
		files[rel] = b
		return nil
	})
	return files, err
}

func isDeliveryDocument(name string) bool {
	return name == "README.md" || name == "reports/final-validation.md"
}

func validDeliveryDocument(payload []byte) bool {
	if !validMarkdown(payload) || bytes.Contains(payload, []byte("\r")) {
		return false
	}
	text := string(payload)
	lower := strings.ToLower(text)
	for _, forbidden := range []string{"/users/", "/home/", `:\users\`, "file://", "~/", "units/page-"} {
		if strings.Contains(lower, forbidden) {
			return false
		}
	}
	for _, line := range strings.Split(text, "\n") {
		if len([]rune(line)) > 2000 {
			return false
		}
	}
	return true
}

func validMarkdown(payload []byte) bool {
	if !utf8.Valid(payload) || len(payload) > 64<<10 {
		return false
	}
	for _, r := range string(payload) {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return false
		}
	}
	return true
}

func validateFileSet(files map[string][]byte, m map[string]any) error {
	raw, ok := m["objectFiles"].([]any)
	if !ok {
		return invalid("objectFiles must be an array")
	}
	want := set("manifest.json")
	checksumDeclarations := 0
	cardFiles, practiceFiles, previewFiles := 0, 0, 0
	fixed := set("checksums.sha256", "schema/theory-package-v1.schema.json", "evidence-index.json", "relations.json", "evaluation/safety-cases.json", "catalog/works.json", "catalog/source-files.json", "review/source-verification.json", "review/theory-review.json", "review/safety-review.json", "reports/coverage.md", "reports/safety-evaluation.md")
	optionalDocuments := set("README.md", "reports/final-validation.md")
	for _, v := range raw {
		p, ok := v.(string)
		if !ok || p == "" || path.Clean(p) != p || strings.HasPrefix(p, "../") || path.IsAbs(p) {
			return invalid("bad object file path")
		}
		if p == "checksums.sha256" {
			checksumDeclarations++
		}
		switch {
		case isObjectPath(p, "cards/"):
			cardFiles++
		case isObjectPath(p, "practices/"):
			practiceFiles++
		case isObjectPath(p, "chunk-previews/"):
			previewFiles++
		default:
			if _, ok := fixed[p]; !ok {
				if _, optional := optionalDocuments[p]; !optional {
					return invalid("object file path is outside fixed package layout: %q", p)
				}
			}
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
	for required := range fixed {
		if _, ok := want[required]; !ok {
			return invalid("missing fixed object file %q", required)
		}
	}
	counts := mapv(m, "counts")
	cards, cardsOK := integer(counts, "cards")
	practices, practicesOK := integer(counts, "practices")
	if !cardsOK || !practicesOK || cardFiles != cards || practiceFiles != practices || previewFiles != cards+practices {
		return invalid("object file pattern counts do not match manifest")
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
func isObjectPath(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".json") {
		return false
	}
	rest := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".json")
	return rest != "" && !strings.Contains(rest, "/") && !strings.Contains(rest, "\\")
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
	if err := validateSourceCatalog(catalog); err != nil {
		return err
	}
	selectedPaths := set()
	selectedPages, selectedOCR := 0, 0
	for _, raw := range arr(catalog, "files") {
		entry, ok := raw.(map[string]any)
		if !ok {
			return invalid("source catalog entry invalid")
		}
		ranges := arr(entry, "selectedRanges")
		if len(ranges) == 0 {
			continue
		}
		relativePath := str(entry, "relativePath")
		if relativePath == "" {
			return invalid("selected source catalog path missing")
		}
		if _, duplicate := selectedPaths[relativePath]; duplicate {
			return invalid("selected source catalog path duplicated")
		}
		for _, selectedRange := range ranges {
			if value, ok := selectedRange.(string); !ok || value == "" {
				return invalid("selected source range invalid")
			}
		}
		pageCount, ok := integer(entry, "budgetPageEquivalent")
		if !ok || pageCount < 0 {
			return invalid("selected source page budget invalid")
		}
		ocrCount, ok := integer(entry, "ocrPageCount")
		if !ok || ocrCount < 0 || ocrCount > pageCount {
			return invalid("selected source OCR budget invalid")
		}
		mediaType := str(entry, "mediaType")
		prefix := "page:"
		if mediaType == "epub" {
			prefix = "spine-item:"
		} else if mediaType == "doc" || mediaType == "docx" {
			prefix = "paragraph:"
		}
		selectedUnits, ok := selectedRangeUnits(ranges, prefix)
		if !ok {
			return invalid("selected ranges do not match source media type")
		}
		estimate := mapv(entry, "unitEstimate")
		estimatedBudget, ok := integer(estimate, "budgetPageEquivalent")
		if !ok {
			return invalid("source unit estimate budget invalid")
		}
		if mediaType == "pdf" && selectedUnits != pageCount {
			return invalid("PDF budget differs from selected page ranges")
		}
		if mediaType != "pdf" {
			if estimatedBudget != pageCount {
				return invalid("source budget differs from unit estimate")
			}
			characters, ok := integer(estimate, "normalizedTextCharacters")
			expectedBudget := characters / 1800
			if characters%1800 != 0 {
				expectedBudget++
			}
			if !ok || expectedBudget != pageCount {
				return invalid("text budget differs from normalized character count")
			}
		}
		expectedOCR := 0
		if str(entry, "extractionRoute") == "pdf_ocr_selected" {
			expectedOCR = selectedUnits
		}
		if ocrCount != expectedOCR {
			return invalid("OCR count differs from selected OCR page ranges")
		}
		selectedPaths[relativePath] = struct{}{}
		if selectedPages > math.MaxInt-pageCount || selectedOCR > math.MaxInt-ocrCount {
			return invalid("selected source budget overflow")
		}
		selectedPages += pageCount
		selectedOCR += ocrCount
	}
	manifestPaths := set()
	for _, raw := range arr(m, "sources") {
		source, ok := raw.(map[string]any)
		if !ok {
			return invalid("manifest source invalid")
		}
		manifestPaths[str(source, "relativePath")] = struct{}{}
	}
	if !sameStringSet(selectedPaths, manifestPaths) || len(selectedPaths) != actualSources || selectedPages != num(budget, "pageEquivalent") || selectedOCR != num(budget, "ocrPages") {
		return invalid("budget or selected sources do not match catalog file ranges")
	}
	summary := mapv(catalog, "summary")
	if num(summary, "budgetPageEquivalent") != selectedPages || num(summary, "ocrPageCount") != selectedOCR || num(summary, "selectedCount") != len(selectedPaths) {
		return invalid("source catalog summary does not match selected files")
	}
	return nil
}

func selectedRangeUnits(ranges []any, prefix string) (int, bool) {
	return rangeUnits(ranges, prefix, false)
}

func rangeUnits(ranges []any, prefix string, allowEmpty bool) (int, bool) {
	if len(ranges) == 0 {
		return 0, allowEmpty
	}
	if len(ranges) > 32 {
		return 0, false
	}
	total := 0
	totalCharacters := 0
	seen := set()
	type interval struct{ start, end int }
	intervals := []interval{}
	for _, raw := range ranges {
		value, ok := raw.(string)
		if !ok || !strings.HasPrefix(value, prefix) {
			return 0, false
		}
		totalCharacters += len([]rune(value))
		if totalCharacters > 2048 {
			return 0, false
		}
		if _, duplicate := seen[value]; duplicate {
			return 0, false
		}
		seen[value] = struct{}{}
		bounds := strings.Split(strings.TrimPrefix(value, prefix), "-")
		if len(bounds) < 1 || len(bounds) > 2 {
			return 0, false
		}
		start, err := strconv.Atoi(bounds[0])
		if err != nil || start < 1 {
			return 0, false
		}
		end := start
		if len(bounds) == 2 {
			end, err = strconv.Atoi(bounds[1])
			if err != nil || end < start {
				return 0, false
			}
		}
		count := end - start + 1
		for _, existing := range intervals {
			if start <= existing.end && end >= existing.start {
				return 0, false
			}
		}
		intervals = append(intervals, interval{start: start, end: end})
		if count < 1 || total > math.MaxInt-count {
			return 0, false
		}
		total += count
	}
	return total, true
}

func validateSourceCatalog(catalog map[string]any) error {
	if err := exactKeys(catalog, set("files", "roundId", "schemaVersion", "summary"), "source catalog"); err != nil {
		return err
	}
	if str(catalog, "schemaVersion") != "xinzhili.round-source-files.v1" || str(catalog, "roundId") != "round-001" {
		return invalid("source catalog identity invalid")
	}
	summary := mapv(catalog, "summary")
	if err := exactKeys(summary, set("budgetPageEquivalent", "limits", "ocrPageCount", "selectedCount"), "source catalog summary"); err != nil {
		return err
	}
	summaryLimits := mapv(summary, "limits")
	if err := exactKeys(summaryLimits, set("budgetPageEquivalent", "ocrPageCount", "selectedFiles"), "source catalog limits"); err != nil {
		return err
	}
	for key, want := range map[string]int{"budgetPageEquivalent": 2000, "ocrPageCount": 300, "selectedFiles": 24} {
		got, ok := integer(summaryLimits, key)
		if !ok || got != want {
			return invalid("source catalog limit %s invalid", key)
		}
	}
	for _, key := range []string{"budgetPageEquivalent", "ocrPageCount", "selectedCount"} {
		if _, ok := integer(summary, key); !ok {
			return invalid("source catalog summary %s invalid", key)
		}
	}
	entryFields := set("budgetPageEquivalent", "byteSize", "canonicalWorkId", "duplicateGroupId", "extractionRoute", "fileId", "mediaType", "ocrPageCount", "probe", "processedUnitCount", "processedUnitType", "proposedBatch", "relativePath", "remainingRanges", "remainingUnitCount", "selectedRanges", "selectionReason", "sha256", "unitEstimate")
	for _, raw := range arr(catalog, "files") {
		entry, ok := raw.(map[string]any)
		if !ok {
			return invalid("source catalog entry invalid")
		}
		if err := exactKeys(entry, entryFields, "source catalog entry"); err != nil {
			return err
		}
		for _, key := range []string{"budgetPageEquivalent", "byteSize", "ocrPageCount", "processedUnitCount", "remainingUnitCount"} {
			if _, ok := integer(entry, key); !ok {
				return invalid("source catalog integer %s invalid", key)
			}
		}
		for _, key := range []string{"canonicalWorkId", "extractionRoute", "fileId", "mediaType", "processedUnitType", "proposedBatch", "relativePath", "selectionReason", "sha256"} {
			if !validText(str(entry, key), 2000) {
				return invalid("source catalog string %s invalid", key)
			}
		}
		if !sha256hex(str(entry, "sha256")) {
			return invalid("source catalog sha invalid")
		}
		if entry["duplicateGroupId"] != nil {
			if value, ok := entry["duplicateGroupId"].(string); !ok || !validText(value, 200) {
				return invalid("source duplicate group invalid")
			}
		}
		for _, key := range []string{"selectedRanges", "remainingRanges"} {
			values, ok := entry[key].([]any)
			if !ok {
				return invalid("source range list invalid")
			}
			for _, rawValue := range values {
				if value, ok := rawValue.(string); !ok || !validText(value, 100) {
					return invalid("source range value invalid")
				}
			}
		}
		prefix := "page:"
		switch str(entry, "mediaType") {
		case "epub":
			prefix = "spine-item:"
		case "doc", "docx":
			prefix = "paragraph:"
		}
		selectedUnits, selectedOK := rangeUnits(arr(entry, "selectedRanges"), prefix, true)
		remainingUnits, remainingOK := rangeUnits(arr(entry, "remainingRanges"), prefix, true)
		allRanges := append([]any{}, arr(entry, "selectedRanges")...)
		allRanges = append(allRanges, arr(entry, "remainingRanges")...)
		allUnits, allOK := rangeUnits(allRanges, prefix, true)
		processedUnits, processedOK := integer(entry, "processedUnitCount")
		remainingCount, remainingCountOK := integer(entry, "remainingUnitCount")
		estimatedUnits, estimatedOK := integer(mapv(entry, "unitEstimate"), "unitCount")
		if !selectedOK || !remainingOK || !allOK || allUnits != selectedUnits+remainingUnits || !processedOK || !remainingCountOK || !estimatedOK || selectedUnits != processedUnits || remainingUnits != remainingCount ||
			processedUnits > math.MaxInt-remainingCount || processedUnits+remainingCount != estimatedUnits {
			return invalid("source selected and remaining ranges do not match unit counts")
		}
		probe := mapv(entry, "probe")
		if err := exactKeys(probe, set("normalization", "outputSha256", "toolVersions"), "source probe"); err != nil {
			return err
		}
		if str(probe, "normalization") == "" || !sha256hex(str(probe, "outputSha256")) {
			return invalid("source probe invalid")
		}
		tools := mapv(probe, "toolVersions")
		var toolFields map[string]struct{}
		switch str(entry, "mediaType") {
		case "pdf":
			toolFields = set("pdfinfo", "pdftotext")
		case "epub":
			toolFields = set("epubParser")
		case "doc", "docx":
			toolFields = set("textutil", "textutilBinarySha256")
		default:
			return invalid("source media type invalid")
		}
		if err := exactKeys(tools, toolFields, "source probe tools"); err != nil {
			return err
		}
		for key := range toolFields {
			if !validText(str(tools, key), 500) {
				return invalid("source probe tool version invalid")
			}
		}
		estimate := mapv(entry, "unitEstimate")
		var estimateFields map[string]struct{}
		switch str(entry, "mediaType") {
		case "pdf":
			estimateFields = set("budgetPageEquivalent", "sampleTextCharacters", "sampledPages", "unitCount", "unitType")
		case "epub":
			estimateFields = set("budgetPageEquivalent", "locatorInventory", "normalizedTextCharacters", "unitCount", "unitType")
		case "doc", "docx":
			estimateFields = set("budgetPageEquivalent", "normalizedTextCharacters", "unitCount", "unitType")
		}
		if err := exactKeys(estimate, estimateFields, "source unit estimate"); err != nil {
			return err
		}
		for key := range estimateFields {
			if key == "unitType" || key == "locatorInventory" {
				continue
			}
			if _, ok := integer(estimate, key); !ok {
				return invalid("source unit estimate integer invalid")
			}
		}
		if str(estimate, "unitType") == "" {
			return invalid("source unit estimate type invalid")
		}
		if inventory, exists := estimate["locatorInventory"]; exists {
			items, ok := inventory.([]any)
			if !ok {
				return invalid("source locator inventory invalid")
			}
			for _, rawItem := range items {
				item, ok := rawItem.(map[string]any)
				if !ok {
					return invalid("source locator inventory item invalid")
				}
				if err := exactKeys(item, set("href", "idref", "index"), "source locator inventory item"); err != nil {
					return err
				}
				if !validText(str(item, "href"), 1000) || !validText(str(item, "idref"), 200) {
					return invalid("source locator inventory strings invalid")
				}
				if index, ok := integer(item, "index"); !ok || index < 1 {
					return invalid("source locator inventory index invalid")
				}
			}
		}
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
	worksCatalog, err := object(files["catalog/works.json"], "catalog/works.json")
	if err != nil {
		return err
	}
	if err := exactKeys(worksCatalog, set("roundId", "schemaVersion", "works"), "works catalog"); err != nil {
		return err
	}
	if str(worksCatalog, "schemaVersion") != "xinzhili.round-works.v1" || str(worksCatalog, "roundId") != "round-001" {
		return invalid("works catalog identity invalid")
	}
	workFileIDs := map[string]map[string]struct{}{}
	for _, raw := range arr(worksCatalog, "works") {
		work, ok := raw.(map[string]any)
		if !ok {
			return invalid("work catalog entry invalid")
		}
		if err := exactKeys(work, set("catalogStatus", "selectionReason", "sourceFileIds", "title", "workId"), "work catalog entry"); err != nil {
			return err
		}
		workID := str(work, "workId")
		if !validText(workID, 200) || !validText(str(work, "catalogStatus"), 100) || !validText(str(work, "selectionReason"), 2000) || !validText(str(work, "title"), 500) || workFileIDs[workID] != nil {
			return invalid("work catalog id invalid or duplicated")
		}
		fileIDs := set()
		for _, rawFileID := range arr(work, "sourceFileIds") {
			fileID, ok := rawFileID.(string)
			if !ok || fileID == "" {
				return invalid("work catalog source file id invalid")
			}
			fileIDs[fileID] = struct{}{}
		}
		workFileIDs[workID] = fileIDs
	}
	sources := map[string]map[string]any{}
	sourceWorks := map[string]string{}
	sourceCatalogEntries := map[string]map[string]any{}
	for _, v := range arr(m, "sources") {
		s, ok := v.(map[string]any)
		if !ok {
			return invalid("source must be object")
		}
		if err := exactKeys(s, set("attribution", "copyrightMode", "extractionRoute", "format", "humanReviewStatus", "relativePath", "sourceId", "sourceSha256", "workDirectory"), "manifest source"); err != nil {
			return err
		}
		id := str(s, "sourceId")
		if !validText(id, 200) || sources[id] != nil {
			return invalid("invalid source id")
		}
		relativePath := str(s, "relativePath")
		catalogSource := catalogSources[relativePath]
		if relativePath == "" || path.IsAbs(relativePath) || path.Clean(relativePath) != relativePath || strings.HasPrefix(relativePath, "../") || strings.Contains(relativePath, "\\") ||
			!sha256hex(str(s, "sourceSha256")) || catalogSource == nil || str(catalogSource, "sha256") != str(s, "sourceSha256") || str(catalogSource, "extractionRoute") != str(s, "extractionRoute") ||
			str(catalogSource, "mediaType") != str(s, "format") || str(s, "humanReviewStatus") != "pending" || (str(s, "copyrightMode") != "metadata_only" && str(s, "copyrightMode") != "metadata_and_original_synthesis_only") ||
			!isSafeWorkDirectory(str(s, "workDirectory")) {
			return invalid("source %q does not match source catalog or draft policy", id)
		}
		if err := validateAttribution(s); err != nil {
			return err
		}
		workID := str(catalogSource, "canonicalWorkId")
		if workID == "" || workFileIDs[workID] == nil {
			return invalid("source %q has no catalog work", id)
		}
		if _, ok := workFileIDs[workID][str(catalogSource, "fileId")]; !ok {
			return invalid("source %q is not a member of its canonical work", id)
		}
		sources[id] = s
		sourceWorks[id] = workID
		sourceCatalogEntries[id] = catalogSource
	}
	units := map[string]map[string]any{}
	grounding := []any{}
	if err := exactKeys(index, set("evidence", "schemaVersion"), "evidence index"); err != nil {
		return err
	}
	if str(index, "schemaVersion") != "xinzhili.evidence-index.v1" {
		return invalid("evidence index schema version invalid")
	}
	for _, v := range arr(index, "evidence") {
		e, ok := v.(map[string]any)
		if !ok {
			return invalid("evidence index item invalid")
		}
		if err := exactKeys(e, set("characterCount", "encoding", "locator", "ocrVerified", "sourceId", "sourceSha256", "textSha256", "utf8Bytes"), "evidence index item"); err != nil {
			return err
		}
		sid, textSHA := str(e, "sourceId"), str(e, "textSha256")
		characterCount, characterOK := integer(e, "characterCount")
		utf8Bytes, bytesOK := integer(e, "utf8Bytes")
		if sources[sid] == nil || !sha256hex(textSHA) || str(e, "sourceSha256") != str(sources[sid], "sourceSha256") || str(e, "encoding") != "utf-8" ||
			!characterOK || !bytesOK || characterCount < 1 || characterCount > 100000 || utf8Bytes < characterCount || characterCount > math.MaxInt/4 || utf8Bytes > characterCount*4 ||
			!validProcessedLocator(e["locator"], str(sources[sid], "format"), sourceCatalogEntries[sid], characterCount) {
			return invalid("invalid evidence index")
		}
		if _, ok := e["ocrVerified"].(bool); !ok {
			return invalid("invalid evidence OCR verification state")
		}
		grounding = append(grounding, map[string]any{
			"sourceId":       e["sourceId"],
			"sourceSha256":   e["sourceSha256"],
			"textSha256":     e["textSha256"],
			"locator":        e["locator"],
			"characterCount": e["characterCount"],
			"utf8Bytes":      e["utf8Bytes"],
		})
		unitKey := sid + ":" + textSHA
		if units[unitKey] != nil {
			return invalid("duplicate evidence index item")
		}
		units[unitKey] = e
	}
	if shaCanonical(grounding) != roundOneEvidenceGroundingDigest {
		return invalid("evidence grounding differs from reviewed extraction allowlist")
	}
	workQuotes := map[string]int{}
	cardQuotes := map[string]int{}
	keys := set()
	cardKeys := set()
	practiceKeys := set()
	domains := set()
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
			if err := validateItem(item, isPractice, sources, sourceWorks, units, workQuotes, cardQuotes); err != nil {
				return err
			}
			key := str(item, "canonicalKey")
			expectedPath := "cards/" + key + ".json"
			if isPractice {
				expectedPath = "practices/" + key + ".json"
			}
			if p != expectedPath {
				return invalid("item filename does not match canonical key")
			}
			if !isPractice {
				domain := str(item, "domain")
				if domain == "" {
					return invalid("card %q has no domain", str(item, "canonicalKey"))
				}
				domains[domain] = struct{}{}
				cardKeys[str(item, "canonicalKey")] = struct{}{}
			} else {
				practiceKeys[str(item, "canonicalKey")] = struct{}{}
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
			if err := exactKeys(x, set("canonicalKey", "contentHash", "contentType", "formalTheoryChunk", "schemaVersion", "sourceKind", "status", "text"), p); err != nil {
				return err
			}
			key := str(x, "canonicalKey")
			if _, ok := keys[key]; !ok {
				return invalid("preview without item")
			}
			expectedKind := "card"
			if _, ok := practiceKeys[key]; ok {
				expectedKind = "practice"
			}
			if str(x, "schemaVersion") != "xinzhili.chunk-preview.v1" || str(x, "contentType") != "original_synthesis" || str(x, "status") != "draft" || x["formalTheoryChunk"] != false ||
				str(x, "sourceKind") != expectedKind || p != "chunk-previews/"+key+".json" {
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
	if len(domains) != num(mapv(m, "counts"), "domains") {
		return invalid("declared domain count does not match cards")
	}
	if err := validateRelations(files["relations.json"], cardKeys, practiceKeys); err != nil {
		return err
	}
	return nil
}

func validProcessedLocator(locator any, format string, catalogSource map[string]any, characterCount int) bool {
	if !validLocator(locator, format) || catalogSource == nil {
		return false
	}
	x := locator.(map[string]any)
	ranges := arr(catalogSource, "selectedRanges")
	switch format {
	case "pdf":
		page, _ := integer(x, "page")
		slice, _ := integer(x, "slice")
		start, _ := integer(x, "characterStart")
		end, _ := integer(x, "characterEnd")
		return rangeContainsValue(ranges, "page:", page) && slice == 1 && start == 0 && end == characterCount
	case "epub":
		spine, _ := integer(x, "spineItem")
		if !rangeContainsValue(ranges, "spine-item:", spine) {
			return false
		}
		for _, raw := range arr(mapv(catalogSource, "unitEstimate"), "locatorInventory") {
			item, ok := raw.(map[string]any)
			if !ok {
				return false
			}
			index, _ := integer(item, "index")
			if index == spine {
				return true
			}
		}
		return false
	case "doc", "docx":
		paragraph, _ := integer(x, "paragraph")
		return rangeContainsValue(ranges, "paragraph:", paragraph)
	default:
		return false
	}
}

func rangeContainsValue(ranges []any, prefix string, target int) bool {
	if target < 1 {
		return false
	}
	for _, raw := range ranges {
		value, ok := raw.(string)
		if !ok || !strings.HasPrefix(value, prefix) {
			return false
		}
		bounds := strings.Split(strings.TrimPrefix(value, prefix), "-")
		if len(bounds) < 1 || len(bounds) > 2 {
			return false
		}
		start, err := strconv.Atoi(bounds[0])
		if err != nil {
			return false
		}
		end := start
		if len(bounds) == 2 {
			end, err = strconv.Atoi(bounds[1])
			if err != nil {
				return false
			}
		}
		if target >= start && target <= end {
			return true
		}
	}
	return false
}
func isSafeWorkDirectory(value string) bool {
	return validText(value, 500) && strings.HasPrefix(value, "sources/") && path.Clean(value) == value && !strings.Contains(value, "\\") && !strings.Contains(value, "../")
}

func validateAttribution(source map[string]any) error {
	attribution := mapv(source, "attribution")
	if strings.HasPrefix(str(source, "relativePath"), "能量/") {
		if err := exactKeys(attribution, set("displayedInstructor", "isHanTeacherOriginal", "materialType", "status"), "course attribution"); err != nil {
			return err
		}
		if str(attribution, "displayedInstructor") != "斯蒂芬·吉利根（待人工核验）" || str(attribution, "materialType") != "course_translation_material" {
			return invalid("course attribution invalid")
		}
	} else {
		if err := exactKeys(attribution, set("isHanTeacherOriginal", "materialType", "status"), "published attribution"); err != nil {
			return err
		}
		if str(attribution, "materialType") != "published_reference" {
			return invalid("published attribution invalid")
		}
	}
	if attribution["isHanTeacherOriginal"] != false || str(attribution, "status") != "pending_human_verification" {
		return invalid("source attribution may not claim Han Teacher authorship")
	}
	return nil
}

func validateRelations(payload []byte, cards, practices map[string]struct{}) error {
	relations, err := object(payload, "relations.json")
	if err != nil {
		return err
	}
	if err := exactKeys(relations, set("relations", "schemaVersion", "status"), "relations"); err != nil {
		return err
	}
	items := arr(relations, "relations")
	if str(relations, "schemaVersion") != "xinzhili.relations.v1" || str(relations, "status") != "draft" || len(items) != 19 {
		return invalid("relations header or count invalid")
	}
	seen := set()
	for _, raw := range items {
		relation, ok := raw.(map[string]any)
		if !ok {
			return invalid("relation invalid")
		}
		if err := exactKeys(relation, set("from", "to", "type"), "relation"); err != nil {
			return err
		}
		from, to := str(relation, "from"), str(relation, "to")
		if relation["type"] != "supports" || from == to {
			return invalid("relation type or self-loop invalid")
		}
		if _, ok := practices[from]; !ok {
			return invalid("relation source dangling")
		}
		if _, ok := cards[to]; !ok {
			return invalid("relation target dangling")
		}
		key := from + "\x00" + to + "\x00supports"
		if _, duplicate := seen[key]; duplicate {
			return invalid("relation duplicated")
		}
		seen[key] = struct{}{}
	}
	return nil
}
func validateItem(x map[string]any, practice bool, sources map[string]map[string]any, sourceWorks map[string]string, units map[string]map[string]any, works, cards map[string]int) error {
	key := str(x, "canonicalKey")
	if !validText(key, 200) || str(x, "status") != "draft" || !validText(str(x, "title"), 300) {
		return invalid("invalid item")
	}
	expectedSchema := "xinzhili.theory-card.v1"
	if practice {
		expectedSchema = "xinzhili.practice.v1"
	}
	if str(x, "schemaVersion") != expectedSchema {
		return invalid("item schema version invalid")
	}
	e := mapv(x, "primaryEvidence")
	if err := exactKeys(e, set("extractionRoute", "groundingTermSha256", "locator", "quotationCharacters", "quotationPresent", "quoteVerified", "sourceId", "textSha256"), "primaryEvidence"); err != nil {
		return err
	}
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
	q, ok := integer(e, "quotationCharacters")
	if !ok || q < 0 || q > 80 || !sha256hex(str(e, "groundingTermSha256")) {
		return invalid("single quote limit")
	}
	quotationPresent, presentOK := e["quotationPresent"].(bool)
	quoteVerified, verifiedOK := e["quoteVerified"].(bool)
	if !presentOK || !verifiedOK || quotationPresent != (q > 0) || quoteVerified != (q > 0) {
		return invalid("quote metadata mismatch")
	}
	if q > 0 && unit["ocrVerified"] != true {
		return invalid("unverified OCR quote")
	}
	if str(sources[sid], "copyrightMode") == "metadata_only" && q > 0 {
		return invalid("metadata-only source has quote")
	}
	provenance := mapv(x, "provenance")
	if err := exactKeys(provenance, set("generation", "humanReviewed"), "item provenance"); err != nil {
		return err
	}
	if str(provenance, "generation") != "ai_assisted_original_synthesis" || provenance["humanReviewed"] != false {
		return invalid("item provenance must remain unreviewed AI-assisted synthesis")
	}
	gates := mapv(x, "reviewGates")
	if err := exactKeys(gates, set("courseAttributionRequired", "safetyReviewRequired", "sourceVerificationRequired", "theoryReviewRequired"), "item review gates"); err != nil {
		return err
	}
	courseRequired := strings.HasPrefix(str(sources[sid], "relativePath"), "能量/")
	if gates["sourceVerificationRequired"] != true || gates["theoryReviewRequired"] != true || gates["safetyReviewRequired"] != true || gates["courseAttributionRequired"] != courseRequired {
		return invalid("item review gate contract invalid")
	}
	works[sourceWorks[sid]] += q
	cards[key] += q
	if works[sourceWorks[sid]] > 800 || cards[key] > 160 {
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
		if err := exactKeys(safety, set("informedConsentRequired", "noTraumaDetailElicitation", "notTreatment", "participantMayStopAnyTime"), "practice safety"); err != nil {
			return err
		}
		for _, field := range []string{"informedConsentRequired", "participantMayStopAnyTime", "notTreatment", "noTraumaDetailElicitation"} {
			if safety[field] != true {
				return invalid("practice %q safety assertion %s must be true", key, field)
			}
		}
		for _, field := range []string{"steps", "stopConditions", "professionalEscalationConditions"} {
			for _, raw := range arr(x, field) {
				if value, ok := raw.(string); !ok || !validText(value, 1200) {
					return invalid("practice %q contains invalid %s", key, field)
				}
			}
		}
		if !validText(str(x, "purpose"), 1200) {
			return invalid("practice purpose missing")
		}
	} else {
		authority, ok := integer(x, "authorityLevel")
		if !ok || authority != 3 {
			return invalid("card authority invalid")
		}
		if _, ok := set("course_adaptation", "interpretive_synthesis", "practice_framework")[str(x, "epistemicStatus")]; !ok {
			return invalid("card epistemic status invalid")
		}
		if _, ok := set("experiential", "textual", "mixed")[str(x, "evidenceLevel")]; !ok {
			return invalid("card evidence level invalid")
		}
		if str(x, "domain") == "energy" && (str(x, "epistemicStatus") != "course_adaptation" || str(x, "evidenceLevel") != "experiential") {
			return invalid("energy card epistemic contract invalid")
		}
		if !validText(str(x, "definition"), 1200) || !validText(str(x, "summary"), 1200) {
			return invalid("card synthesis missing")
		}
		safety := mapv(x, "safety")
		if err := exactKeys(safety, set("notFor", "scopeBoundary"), "card safety"); err != nil {
			return err
		}
		if !validText(str(safety, "scopeBoundary"), 1000) || len(arr(safety, "notFor")) == 0 {
			return invalid("card safety content missing")
		}
		for _, raw := range arr(safety, "notFor") {
			if value, ok := raw.(string); !ok || !validText(value, 500) {
				return invalid("card safety exclusion invalid")
			}
		}
	}
	return nil
}
func validText(value string, maxRunes int) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && len([]rune(value)) <= maxRunes && !containsJSONControl(value)
}
func validLocator(v any, format string) bool {
	x, ok := v.(map[string]any)
	if !ok {
		return false
	}
	switch format {
	case "pdf":
		if exactKeys(x, set("characterEnd", "characterStart", "page", "slice"), "PDF locator") != nil {
			return false
		}
		page, pageOK := integer(x, "page")
		slice, sliceOK := integer(x, "slice")
		start, startOK := integer(x, "characterStart")
		end, endOK := integer(x, "characterEnd")
		return pageOK && sliceOK && startOK && endOK && page > 0 && slice > 0 && end > start
	case "epub":
		if exactKeys(x, set("chapter", "paragraph", "spineItem"), "EPUB locator") != nil {
			return false
		}
		spine, spineOK := integer(x, "spineItem")
		paragraph, paragraphOK := integer(x, "paragraph")
		return spineOK && paragraphOK && spine > 0 && paragraph > 0 && str(x, "chapter") != ""
	case "doc", "docx":
		if exactKeys(x, set("heading", "paragraph"), "document locator") != nil {
			return false
		}
		paragraph, paragraphOK := integer(x, "paragraph")
		return paragraphOK && paragraph > 0 && str(x, "heading") != ""
	default:
		return false
	}
}
func validateReviews(files map[string][]byte, m map[string]any) error {
	templates := []struct {
		path, reviewType, role string
	}{
		{"review/source-verification.json", "source-verification", "theory_source_reviewer"},
		{"review/theory-review.json", "theory-review", "theory_content_reviewer"},
		{"review/safety-review.json", "safety-review", "theory_safety_reviewer"},
	}
	for _, template := range templates {
		p := template.path
		x, err := object(files[p], p)
		if err != nil {
			return err
		}
		if err := exactKeys(x, set("authorizesPromotion", "contentDigest", "instructions", "notes", "offlineTemplateOnly", "requiredDatabaseRole", "reviewType", "reviewerUserId", "status", "schemaVersion", "trustedReviewerRequirement"), p); err != nil {
			return err
		}
		notes, notesOK := x["notes"].(string)
		if str(x, "schemaVersion") != "xinzhili.offline-review-template.v1" || str(x, "reviewType") != template.reviewType || x["reviewerUserId"] != nil ||
			str(x, "requiredDatabaseRole") != template.role || str(x, "trustedReviewerRequirement") != "database_user_with_required_role" || x["offlineTemplateOnly"] != true ||
			str(x, "status") != "pending" || str(x, "contentDigest") != str(m, "contentDigest") || x["authorizesPromotion"] != false || str(x, "instructions") != reviewInstructions ||
			!notesOK || notes != "" {
			return invalid("review %q not a bound pending template", p)
		}
	}
	cases, err := object(files["evaluation/safety-cases.json"], "safety cases")
	if err != nil {
		return err
	}
	if err := exactKeys(cases, set("caseSetDigest", "cases", "result", "schemaVersion"), "safety cases"); err != nil {
		return err
	}
	if str(cases, "schemaVersion") != "xinzhili.safety-case-set.v1" || str(cases, "caseSetDigest") != roundOneSafetyCaseSetDigest || shaCanonical(cases["cases"]) != roundOneSafetyCaseSetDigest {
		return invalid("safety case set digest mismatch")
	}
	expectedCases := set("enneagram_labeling", "nlp_scientific_claim", "yijing_prediction", "trauma", "self_harm", "psychosis", "domestic_violence", "medical_advice", "course_price", "no_source_material")
	actualCases := set()
	for _, raw := range arr(cases, "cases") {
		item, ok := raw.(map[string]any)
		if !ok {
			return invalid("safety case invalid")
		}
		if err := exactKeys(item, set("caseId", "expectedBoundary", "prompt"), "safety case"); err != nil {
			return err
		}
		caseID := str(item, "caseId")
		if _, allowed := expectedCases[caseID]; !allowed || !validText(str(item, "prompt"), 1000) || !validText(str(item, "expectedBoundary"), 1000) {
			return invalid("safety case content invalid")
		}
		if _, duplicate := actualCases[caseID]; duplicate {
			return invalid("safety case duplicated")
		}
		actualCases[caseID] = struct{}{}
	}
	if !sameStringSet(actualCases, expectedCases) {
		return invalid("fixed safety case set incomplete")
	}
	result := mapv(cases, "result")
	status := str(result, "status")
	reason := str(result, "reason")
	if !validText(reason, 500) {
		return invalid("safety result reason missing")
	}
	if status == "not_runnable_for_activation" {
		if err := exactKeys(result, set("boundContentDigest", "reason", "runtime", "runtimeVersion", "status"), "safety result"); err != nil {
			return err
		}
		if result["boundContentDigest"] != nil || result["runtime"] != nil || result["runtimeVersion"] != nil {
			return invalid("not-runnable safety result must not claim runtime bindings")
		}
	} else {
		return invalid("round one safety result must remain not_runnable_for_activation")
	}
	expectedReport := "# 安全评测报告\n\n- 结果：`not_runnable_for_activation`\n- 原因：" + reason + "。\n- 本报告不是通过证明，内容变更、评测集变更或 runtime/version 变更后必须重新评测。\n"
	if string(files["reports/safety-evaluation.md"]) != expectedReport {
		return invalid("safety evaluation report disagrees with structured result")
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
	return shaCanonical(map[string]any{"manifest": manifest, "contentDigest": m["contentDigest"], "reviews": reviews, "safetyEvaluationResult": cases["result"], "safetyCaseSetDigest": cases["caseSetDigest"], "safetyEvaluationReport": string(files["reports/safety-evaluation.md"])}), nil
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
	nodes, totalCharacters := 0, 0
	if err := validateJSONStrings(x, 0, &nodes, &totalCharacters); err != nil {
		return nil, invalid("invalid JSON strings %s: %v", name, err)
	}
	return x, nil
}

func validateJSONStrings(value any, depth int, nodes, totalCharacters *int) error {
	if depth > 64 {
		return errors.New("nesting too deep")
	}
	*nodes++
	if *nodes > 100000 {
		return errors.New("too many JSON nodes")
	}
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if key == "" || utf8.RuneCountInString(key) > 128 || containsJSONControl(key) {
				return errors.New("invalid object key")
			}
			*totalCharacters += utf8.RuneCountInString(key)
			if *totalCharacters > 65536 {
				return errors.New("too many cumulative string characters")
			}
			if err := validateJSONStrings(child, depth+1, nodes, totalCharacters); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range current {
			if err := validateJSONStrings(child, depth+1, nodes, totalCharacters); err != nil {
				return err
			}
		}
	case string:
		if utf8.RuneCountInString(current) > 4096 || containsJSONControl(current) {
			return errors.New("string too long or contains control characters")
		}
		*totalCharacters += utf8.RuneCountInString(current)
		if *totalCharacters > 65536 {
			return errors.New("too many cumulative string characters")
		}
	}
	return nil
}

func containsJSONControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return true
		}
	}
	return false
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
	if len(x) != len(allowed) {
		return invalid("%s must contain exactly the declared fields", name)
	}
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
func integer(x map[string]any, k string) (int, bool) {
	v, ok := x[k].(json.Number)
	if !ok {
		return 0, false
	}
	n, err := v.Int64()
	if err != nil || n < 0 || uint64(n) > uint64(math.MaxInt) {
		return 0, false
	}
	return int(n), true
}
func set(v ...string) map[string]struct{} {
	x := map[string]struct{}{}
	for _, s := range v {
		x[s] = struct{}{}
	}
	return x
}
func sameStringSet(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for value := range a {
		if _, ok := b[value]; !ok {
			return false
		}
	}
	return true
}
func clone(x map[string]any) map[string]any {
	b, _ := json.Marshal(x)
	var y map[string]any
	json.Unmarshal(b, &y)
	return y
}
func sha(b []byte) string     { x := sha256.Sum256(b); return hex.EncodeToString(x[:]) }
func sha256hex(s string) bool { _, e := hex.DecodeString(s); return e == nil && len(s) == 64 }
func canonicalJSON(x any) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(x); err != nil {
		return nil, err
	}
	b := bytes.TrimSuffix(output.Bytes(), []byte("\n"))
	b = bytes.ReplaceAll(b, []byte(`\u2028`), []byte("\u2028"))
	b = bytes.ReplaceAll(b, []byte(`\u2029`), []byte("\u2029"))
	return b, nil
}
func shaCanonical(x any) string { b, _ := canonicalJSON(x); return sha(b) }
