package enterprisepromotion

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestTopicFixedKeysAndStableSort(t *testing.T) {
	topics := FixedTopics()
	want := []TopicKey{TopicTeamCommunication, TopicLeadership, TopicCohesion, TopicCulture, TopicEmployeeGrowth}
	if len(topics) != len(want) {
		t.Fatalf("got %d topics", len(topics))
	}
	for i := range want {
		if topics[i].Key != want[i] || topics[i].SortOrder != i {
			t.Fatalf("topic %d = %+v", i, topics[i])
		}
	}
}

func TestPublicProjectionDoesNotLeakPersistenceFields(t *testing.T) {
	caseRecord := TrainingCase{ID: 7, Slug: "case", Title: "Case", CompanyDisplayName: "某企业", CompanyInternalNameEncrypted: []byte("secret")}
	public := projectPublicCase(caseRecord, []CaseMedia{{ID: 9, CaseID: 7, MediaAssetID: 3, Role: MediaPromo, Status: CaseMediaPublished}}, nil)
	raw, err := json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"secret", "companyInternalNameEncrypted", "consent", "sourceAsset"} {
		if containsJSON(raw, forbidden) {
			t.Fatalf("public JSON leaked %q: %s", forbidden, raw)
		}
	}
	typ := reflect.TypeOf(public)
	for _, forbidden := range []string{"CompanyInternalNameEncrypted", "EvidenceAssetID", "SourceObjectKey"} {
		if _, ok := typ.FieldByName(forbidden); ok {
			t.Fatalf("public projection exposes %s", forbidden)
		}
	}
	mediaField, ok := typ.FieldByName("Media")
	if !ok {
		t.Fatal("public media missing")
	}
	mediaType := mediaField.Type.Elem()
	if mediaType.Name() == "CaseMedia" {
		t.Fatal("public projection reuses persistence media model")
	}
	if _, ok := mediaType.FieldByName("CaseID"); ok {
		t.Fatal("public media exposes persistence case id")
	}
	topicField, ok := typ.FieldByName("Topics")
	if !ok {
		t.Fatal("public topics missing")
	}
	if topicField.Type.Elem().Name() == "TrainingTopic" {
		t.Fatal("public DTO reuses persistence topic model")
	}
	topicJSON, err := json.Marshal(PublicTopic{Key: TopicLeadership, Title: "领导力"})
	if err != nil {
		t.Fatal(err)
	}
	if string(topicJSON) != `{"key":"leadership","title":"领导力"}` {
		t.Fatalf("topic JSON=%s", topicJSON)
	}
}

func TestPublicStoreUsesBatchQueries(t *testing.T) {
	raw, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	if !strings.Contains(src, "ANY($1)") {
		t.Fatal("public associations are not batch loaded")
	}
}

func containsJSON(raw []byte, value string) bool {
	for i := 0; i+len(value) <= len(raw); i++ {
		if string(raw[i:i+len(value)]) == value {
			return true
		}
	}
	return false
}
