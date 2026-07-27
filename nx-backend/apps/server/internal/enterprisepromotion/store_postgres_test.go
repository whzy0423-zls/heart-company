package enterprisepromotion

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/testdb"
)

func openPostgresStore(t *testing.T) (*SQLStore, context.Context) {
	t.Helper()
	database, _ := testdb.OpenEnvIsolatedSchema(t, "enterprise_store")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	if _, err := database.ExecContext(ctx, `CREATE TABLE users(id BIGSERIAL PRIMARY KEY,username TEXT NOT NULL UNIQUE,password_hash TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	const marker = "-- ----- 企业培训推广：媒体、案例、方案、授权、线索和基础归因 -----"
	start := strings.Index(string(raw), marker)
	if start < 0 {
		t.Fatal("enterprise migration marker missing")
	}
	if _, err = database.ExecContext(ctx, string(raw[start:])); err != nil {
		t.Fatal(err)
	}
	return NewStore(database), ctx
}

func seedTrainer(t *testing.T, s *SQLStore, ctx context.Context, key string) EnterpriseTrainer {
	t.Helper()
	v, err := s.CreateTrainer(ctx, EnterpriseTrainer{Key: key, Name: "韩老师", Status: CasePublished})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func seedMediaAsset(t *testing.T, s *SQLStore, ctx context.Context, key, kind string, ready bool) int64 {
	t.Helper()
	sha := strings.Repeat("a", 64)
	state := "reserved"
	if ready {
		state = "qa_pending"
	}
	var id int64
	if err := s.db.QueryRowContext(ctx, `INSERT INTO promotion_media_assets(asset_key,kind,object_key,sha256,state) VALUES($1,$2,$3,$4,$5) RETURNING id`, key, kind, "promotion/"+key, sha, state).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if !ready {
		return id
	}
	var reviewer, attempt int64
	if err := s.db.QueryRowContext(ctx, `INSERT INTO users(username,password_hash) VALUES($1,'x') RETURNING id`, "reviewer-"+key).Scan(&reviewer); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, `INSERT INTO promotion_media_processing_attempts(asset_id,attempt_number,state,output_object_key,output_sha256,finished_at) VALUES($1,1,'succeeded',$2,$3,now()) RETURNING id`, id, "promotion/derived/"+key, sha).Scan(&attempt); err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var review int64
	var approved time.Time
	if err = tx.QueryRowContext(ctx, `INSERT INTO promotion_media_qa_reviews(asset_id,attempt_id,qa_result,approved_by,qa_note) VALUES($1,$2,'passed',$3,'public release') RETURNING id,approved_at`, id, attempt, reviewer).Scan(&review, &approved); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE promotion_media_assets SET state='ready',ready_qa_review_id=$2,ready_attempt_id=$3,qa_result='passed',qa_approved_by=$4,qa_approved_at=$5,qa_note='public release' WHERE id=$1`, id, review, attempt, reviewer, approved); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPostgresTrainerTopicCRUDStableSortAndRestrict(t *testing.T) {
	s, ctx := openPostgresStore(t)
	if err := s.UpsertFixedTopics(ctx); err != nil {
		t.Fatal(err)
	}
	topics, err := s.ListTopics(ctx, true)
	if err != nil || len(topics) != 5 {
		t.Fatalf("topics=%+v err=%v", topics, err)
	}
	trainer := seedTrainer(t, s, ctx, "han")
	trainer.Title = "企业培训师"
	trainer, err = s.UpdateTrainer(ctx, trainer, trainer.Version)
	if err != nil || trainer.Version != 2 {
		t.Fatalf("trainer=%+v err=%v", trainer, err)
	}
	if _, err = s.GetTrainer(ctx, trainer.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteTopic(ctx, topics[0].ID); err != nil {
		t.Fatal(err)
	}

	caseAgg, err := s.CreateCase(ctx, CaseAggregate{Case: baseCase(trainer, "referenced"), TopicIDs: []int64{topics[1].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteTopic(ctx, topics[1].ID); !errors.Is(err, ErrRestricted) {
		t.Fatalf("topic delete=%v", err)
	}
	if err = s.DeleteTrainer(ctx, trainer.ID); !errors.Is(err, ErrRestricted) {
		t.Fatalf("trainer delete=%v", err)
	}
	if err = s.DeleteCase(ctx, caseAgg.Case.ID); !errors.Is(err, ErrRestricted) {
		t.Fatalf("referenced case delete=%v", err)
	}
	unreferenced, err := s.CreateCase(ctx, CaseAggregate{Case: baseCase(trainer, "unreferenced-draft")})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteCase(ctx, unreferenced.Case.ID); err != nil {
		t.Fatalf("unreferenced draft delete=%v", err)
	}
}

func baseCase(trainer EnterpriseTrainer, slug string) TrainingCase {
	return TrainingCase{Slug: slug, Title: "企业沟通案例", TrainerID: trainer.ID, TrainerNameSnapshot: trainer.Name, Status: CaseDraft, AuthorizationStatus: AuthorizationPending, BusinessChallenges: []string{}, TrainingGoals: []string{}, TrainingModules: []string{}, TrainingMethods: []string{}}
}

func TestPostgresCaseStoreOrderedRelationsConflictAndRollback(t *testing.T) {
	s, ctx := openPostgresStore(t)
	_ = s.UpsertFixedTopics(ctx)
	topics, _ := s.ListTopics(ctx, true)
	trainer := seedTrainer(t, s, ctx, "case-trainer")
	var media1, media2 int64
	for i, dst := range []*int64{&media1, &media2} {
		err := s.db.QueryRowContext(ctx, `INSERT INTO promotion_media_assets(asset_key,kind,object_key,sha256) VALUES($1,'video',$2,$3) RETURNING id`,
			[]string{"asset-one", "asset-two"}[i], []string{"promotion/one.mp4", "promotion/two.mp4"}[i], strings.Repeat([]string{"a", "b"}[i], 64)).Scan(dst)
		if err != nil {
			t.Fatal(err)
		}
	}
	a, err := s.CreateCase(ctx, CaseAggregate{Case: baseCase(trainer, "case-a"), TopicIDs: []int64{topics[2].ID, topics[0].ID}, Media: []CaseMedia{{MediaAssetID: media2, Role: MediaHighlight, Caption: "second", Status: CaseMediaPublished}, {MediaAssetID: media1, Role: MediaPromo, Caption: "first", Status: CaseMediaDraft}}})
	if err != nil {
		t.Fatal(err)
	}
	if a.Media[1].CaseID != a.Case.ID || a.Media[1].Position != 1 {
		t.Fatalf("returned aggregate not normalized: %+v", a.Media)
	}
	got, err := s.GetCase(ctx, a.Case.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TopicIDs[0] != topics[2].ID || got.TopicIDs[1] != topics[0].ID {
		t.Fatalf("topic order=%v", got.TopicIDs)
	}
	if len(got.Media) != 2 || got.Media[0].MediaAssetID != media2 || got.Media[1].Position != 1 {
		t.Fatalf("media order=%+v", got.Media)
	}
	replaced, err := s.ReplaceCaseMedia(ctx, got.Case.ID, got.Case.Version, []CaseMedia{{MediaAssetID: media1, Role: MediaPromo, Caption: "only", Status: CaseMediaPublished}})
	if err != nil {
		t.Fatal(err)
	}
	got = replaced
	first := got
	second := got
	first.Case.Title = "winner"
	if _, err = s.UpdateCase(ctx, first, got.Case.Version); err != nil {
		t.Fatal(err)
	}
	second.Case.Title = "loser"
	if _, err = s.UpdateCase(ctx, second, got.Case.Version); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("conflict=%v", err)
	}
	current, _ := s.GetCase(ctx, a.Case.ID)
	before := current.Case.Title
	current.Case.Title = "must rollback"
	current.TopicIDs = []int64{99999999}
	if _, err = s.UpdateCase(ctx, current, current.Case.Version); err == nil {
		t.Fatal("invalid aggregate update succeeded")
	}
	after, _ := s.GetCase(ctx, a.Case.ID)
	if after.Case.Title != before {
		t.Fatalf("rollback failed: %q", after.Case.Title)
	}
}

func TestPostgresSolutionStoreCRUDOrderingConflict(t *testing.T) {
	s, ctx := openPostgresStore(t)
	trainer := seedTrainer(t, s, ctx, "solution-trainer")
	c1, _ := s.CreateCase(ctx, CaseAggregate{Case: baseCase(trainer, "c1")})
	c2, _ := s.CreateCase(ctx, CaseAggregate{Case: baseCase(trainer, "c2")})
	v := EnterpriseSolution{Slug: "communication", Title: "沟通方案", TrainerID: trainer.ID, TrainerNameSnapshot: trainer.Name, Status: CaseDraft, Audiences: []string{}, Problems: []string{}, Goals: []string{}, Modules: []string{}, DeliveryMethods: []string{}, CustomizableItems: []string{}}
	a, err := s.CreateSolution(ctx, SolutionAggregate{Solution: v, CaseIDs: []int64{c2.Case.ID, c1.Case.ID}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSolution(ctx, a.Solution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CaseIDs[0] != c2.Case.ID {
		t.Fatalf("case order=%v", got.CaseIDs)
	}
	copy := got
	got.Solution.Title = "updated"
	updated, err := s.UpdateSolution(ctx, got, got.Solution.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdateSolution(ctx, copy, copy.Solution.Version); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("conflict=%v", err)
	}
	if updated.Solution.Version != 2 {
		t.Fatalf("version=%d", updated.Solution.Version)
	}
	if err = s.DeleteSolution(ctx, a.Solution.ID); !errors.Is(err, ErrRestricted) {
		t.Fatalf("referenced solution delete=%v", err)
	}
	unreferenced, err := s.CreateSolution(ctx, SolutionAggregate{Solution: EnterpriseSolution{Slug: "unreferenced", Title: "未引用", TrainerID: trainer.ID, TrainerNameSnapshot: trainer.Name, Status: CaseDraft, Audiences: []string{}, Problems: []string{}, Goals: []string{}, Modules: []string{}, DeliveryMethods: []string{}, CustomizableItems: []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteSolution(ctx, unreferenced.Solution.ID); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreMapsInvalidReferencesAndMissingVersionsPrecisely(t *testing.T) {
	s, ctx := openPostgresStore(t)
	trainer := seedTrainer(t, s, ctx, "precise-errors")
	bad := baseCase(trainer, "bad-trainer")
	bad.TrainerID = 99999999
	if _, err := s.CreateCase(ctx, CaseAggregate{Case: bad}); !errors.Is(err, ErrInvalidReference) || errors.Is(err, ErrRestricted) {
		t.Fatalf("invalid trainer err=%v", err)
	}
	valid, err := s.CreateCase(ctx, CaseAggregate{Case: baseCase(trainer, "invalid-update-ref")})
	if err != nil {
		t.Fatal(err)
	}
	invalidUpdate := valid
	invalidUpdate.Case.TrainerID = 99999998
	if _, err = s.UpdateCase(ctx, invalidUpdate, valid.Case.Version); !errors.Is(err, ErrInvalidReference) || errors.Is(err, ErrRestricted) {
		t.Fatalf("invalid update reference=%v", err)
	}
	a, err := s.CreateCase(ctx, CaseAggregate{Case: baseCase(trainer, "missing-update")})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteCase(ctx, a.Case.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdateCase(ctx, a, a.Case.Version); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing case update=%v", err)
	}
	v := EnterpriseSolution{Slug: "missing-solution", Title: "方案", TrainerID: trainer.ID, TrainerNameSnapshot: trainer.Name, Status: CaseDraft, Audiences: []string{}, Problems: []string{}, Goals: []string{}, Modules: []string{}, DeliveryMethods: []string{}, CustomizableItems: []string{}}
	badSolution := v
	badSolution.Slug = "bad-solution-ref"
	badSolution.TrainerID = 99999997
	if _, err = s.CreateSolution(ctx, SolutionAggregate{Solution: badSolution}); !errors.Is(err, ErrInvalidReference) || errors.Is(err, ErrRestricted) {
		t.Fatalf("invalid solution reference=%v", err)
	}
	sol, err := s.CreateSolution(ctx, SolutionAggregate{Solution: v})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteSolution(ctx, sol.Solution.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdateSolution(ctx, sol, sol.Solution.Version); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing solution update=%v", err)
	}
}

func TestPostgresGetCaseHonorsCanceledContext(t *testing.T) {
	s, ctx := openPostgresStore(t)
	trainer := seedTrainer(t, s, ctx, "cancelled")
	a, _ := s.CreateCase(ctx, CaseAggregate{Case: baseCase(trainer, "cancelled-case")})
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.GetCase(cancelled, a.Case.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled GetCase=%v", err)
	}
}

func TestPostgresConcurrentCaseUpdatesHaveOneWinner(t *testing.T) {
	s, ctx := openPostgresStore(t)
	trainer := seedTrainer(t, s, ctx, "concurrent")
	a, _ := s.CreateCase(ctx, CaseAggregate{Case: baseCase(trainer, "concurrent-case")})
	base, _ := s.GetCase(ctx, a.Case.ID)
	var wg sync.WaitGroup
	wg.Add(2)
	errs := make(chan error, 2)
	for _, title := range []string{"one", "two"} {
		go func(title string) {
			defer wg.Done()
			v := base
			v.Case.Title = title
			_, e := s.UpdateCase(ctx, v, base.Case.Version)
			errs <- e
		}(title)
	}
	wg.Wait()
	close(errs)
	wins, conflicts := 0, 0
	for e := range errs {
		if e == nil {
			wins++
		} else if errors.Is(e, ErrVersionConflict) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("wins=%d conflicts=%d", wins, conflicts)
	}
}

func TestPostgresBlockedCaseUpdateMapsCommittedOverlapToVersionConflict(t *testing.T) {
	s, ctx := openPostgresStore(t)
	trainer := seedTrainer(t, s, ctx, "blocked-case")
	a, err := s.CreateCase(ctx, CaseAggregate{Case: baseCase(trainer, "blocked-case")})
	if err != nil {
		t.Fatal(err)
	}
	locker, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = locker.ExecContext(ctx, `UPDATE training_cases SET title='external winner',version=version+1 WHERE id=$1`, a.Case.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		v := a
		v.Case.Title = "store loser"
		_, e := s.UpdateCase(context.Background(), v, a.Case.Version)
		done <- e
	}()
	time.Sleep(150 * time.Millisecond)
	if err = locker.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if !errors.Is(err, ErrVersionConflict) {
			t.Fatalf("blocked case update=%v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocked case update did not finish")
	}
}

func TestPostgresBlockedSolutionUpdateMapsCommittedOverlapToVersionConflict(t *testing.T) {
	s, ctx := openPostgresStore(t)
	trainer := seedTrainer(t, s, ctx, "blocked-solution")
	v := EnterpriseSolution{Slug: "blocked-solution", Title: "方案", TrainerID: trainer.ID, TrainerNameSnapshot: trainer.Name, Status: CaseDraft, Audiences: []string{}, Problems: []string{}, Goals: []string{}, Modules: []string{}, DeliveryMethods: []string{}, CustomizableItems: []string{}}
	a, err := s.CreateSolution(ctx, SolutionAggregate{Solution: v})
	if err != nil {
		t.Fatal(err)
	}
	locker, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = locker.ExecContext(ctx, `UPDATE enterprise_solutions SET title='external winner',version=version+1 WHERE id=$1`, a.Solution.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		candidate := a
		candidate.Solution.Title = "store loser"
		_, e := s.UpdateSolution(context.Background(), candidate, a.Solution.Version)
		done <- e
	}()
	time.Sleep(150 * time.Millisecond)
	if err = locker.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if !errors.Is(err, ErrVersionConflict) {
			t.Fatalf("blocked solution update=%v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocked solution update did not finish")
	}
}

func TestPostgresPublicProjectionPublishedOnlyStableTopicFilter(t *testing.T) {
	s, ctx := openPostgresStore(t)
	_ = s.UpsertFixedTopics(ctx)
	topics, _ := s.ListTopics(ctx, true)
	trainer := seedTrainer(t, s, ctx, "public")
	cover := seedMediaAsset(t, s, ctx, "public-cover", "image", true)
	for i, status := range []CaseStatus{CasePublished, CasePublished, CaseDraft} {
		c := baseCase(trainer, []string{"pub-b", "pub-a", "draft"}[i])
		c.Status = status
		c.AuthorizationStatus = AuthorizationApproved
		c.SortOrder = []int{2, 1, 0}[i]
		c.CompanyInternalNameEncrypted = []byte("classified")
		c.CoverAssetID = cover
		video := seedMediaAsset(t, s, ctx, fmt.Sprintf("public-video-%d", i), "video", true)
		if _, e := s.CreateCase(ctx, CaseAggregate{Case: c, TopicIDs: []int64{topics[0].ID}, Media: []CaseMedia{{MediaAssetID: video, Role: MediaPromo, Status: CaseMediaPublished}}}); e != nil {
			t.Fatal(e)
		}
	}
	items, err := s.ListPublicCases(ctx, PublicCaseQuery{Topic: TopicTeamCommunication, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Slug != "pub-a" || items[1].Slug != "pub-b" {
		t.Fatalf("public order=%+v", items)
	}
}

func TestPostgresPublicProjectionWorksWithSingleConnection(t *testing.T) {
	s, ctx := openPostgresStore(t)
	trainer := seedTrainer(t, s, ctx, "single-connection")
	c := baseCase(trainer, "single-public")
	c.Status = CasePublished
	c.AuthorizationStatus = AuthorizationApproved
	c.CoverAssetID = seedMediaAsset(t, s, ctx, "single-cover", "image", true)
	video := seedMediaAsset(t, s, ctx, "single-video", "video", true)
	if _, err := s.CreateCase(ctx, CaseAggregate{Case: c, Media: []CaseMedia{{MediaAssetID: video, Role: MediaPromo, Status: CaseMediaPublished}}}); err != nil {
		t.Fatal(err)
	}
	s.db.SetMaxOpenConns(1)
	short, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	items, err := s.ListPublicCases(short, PublicCaseQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items=%d", len(items))
	}
}

func TestPostgresPublicProjectionRequiresReadyCoverAndPromoVideo(t *testing.T) {
	s, ctx := openPostgresStore(t)
	trainer := seedTrainer(t, s, ctx, "eligibility")
	readyCover := seedMediaAsset(t, s, ctx, "eligible-cover", "image", true)
	readyVideo := seedMediaAsset(t, s, ctx, "eligible-video", "video", true)
	notReadyVideo := seedMediaAsset(t, s, ctx, "not-ready-video", "video", false)
	readyImage := seedMediaAsset(t, s, ctx, "ready-image", "image", true)
	cases := []struct {
		slug    string
		cover   int64
		media   []CaseMedia
		visible bool
	}{
		{slug: "no-cover", media: []CaseMedia{{MediaAssetID: readyVideo, Role: MediaPromo, Status: CaseMediaPublished}}},
		{slug: "no-hero-video", cover: readyCover, media: []CaseMedia{{MediaAssetID: readyVideo, Role: MediaGallery, Status: CaseMediaPublished}}},
		{slug: "video-not-ready", cover: readyCover, media: []CaseMedia{{MediaAssetID: notReadyVideo, Role: MediaPromo, Status: CaseMediaPublished}}},
		{slug: "wrong-cover-kind", cover: readyVideo, media: []CaseMedia{{MediaAssetID: readyVideo, Role: MediaPromo, Status: CaseMediaPublished}}},
		{slug: "wrong-video-kind", cover: readyCover, media: []CaseMedia{{MediaAssetID: readyImage, Role: MediaPromo, Status: CaseMediaPublished}}},
		{slug: "eligible", cover: readyCover, visible: true, media: []CaseMedia{{MediaAssetID: readyVideo, Role: MediaPromo, Status: CaseMediaPublished}, {MediaAssetID: notReadyVideo, Role: MediaHighlight, Status: CaseMediaPublished}, {MediaAssetID: readyVideo, Role: MediaGallery, Status: CaseMediaDraft}}},
	}
	for _, tt := range cases {
		c := baseCase(trainer, tt.slug)
		c.Status = CasePublished
		c.AuthorizationStatus = AuthorizationApproved
		c.CoverAssetID = tt.cover
		if _, err := s.CreateCase(ctx, CaseAggregate{Case: c, Media: tt.media}); err != nil {
			t.Fatal(err)
		}
	}
	items, err := s.ListPublicCases(ctx, PublicCaseQuery{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Slug != "eligible" {
		t.Fatalf("visible cases=%+v", items)
	}
	if len(items[0].Media) != 1 || items[0].Media[0].MediaAssetID != readyVideo {
		t.Fatalf("public media leaked ineligible assets: %+v", items[0].Media)
	}
	if _, err = s.GetPublicCaseBySlug(ctx, "no-cover"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ineligible detail err=%v", err)
	}
	detail, err := s.GetPublicCaseBySlug(ctx, "eligible")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Media) != 1 {
		t.Fatalf("detail media=%+v", detail.Media)
	}
}

func TestPostgresPublicPaginationCapsAtFiftyAndIsStable(t *testing.T) {
	s, ctx := openPostgresStore(t)
	trainer := seedTrainer(t, s, ctx, "pagination")
	cover := seedMediaAsset(t, s, ctx, "pagination-cover", "image", true)
	video := seedMediaAsset(t, s, ctx, "pagination-video", "video", true)
	for i := 0; i < 52; i++ {
		c := baseCase(trainer, fmt.Sprintf("page-%02d", i))
		c.Status = CasePublished
		c.AuthorizationStatus = AuthorizationApproved
		c.CoverAssetID = cover
		c.SortOrder = 7
		if _, err := s.CreateCase(ctx, CaseAggregate{Case: c, Media: []CaseMedia{{MediaAssetID: video, Role: MediaPromo, Status: CaseMediaPublished}}}); err != nil {
			t.Fatal(err)
		}
	}
	page50, err := s.ListPublicCases(ctx, PublicCaseQuery{Limit: 50, Offset: -4})
	if err != nil {
		t.Fatal(err)
	}
	if len(page50) != 50 {
		t.Fatalf("limit50=%d", len(page50))
	}
	capped, err := s.ListPublicCases(ctx, PublicCaseQuery{Limit: 51})
	if err != nil {
		t.Fatal(err)
	}
	if len(capped) != 50 {
		t.Fatalf("limit51=%d", len(capped))
	}
	page1, _ := s.ListPublicCases(ctx, PublicCaseQuery{Limit: 10})
	page2, _ := s.ListPublicCases(ctx, PublicCaseQuery{Limit: 10, Offset: 10})
	if page1[9].ID == page2[0].ID {
		t.Fatal("pages overlap")
	}
	for i := 1; i < len(page1); i++ {
		if page1[i-1].ID <= page1[i].ID {
			t.Fatalf("unstable id order: %d then %d", page1[i-1].ID, page1[i].ID)
		}
	}
}

func TestPostgresPublicListUsesThreeQueriesAndOneSnapshot(t *testing.T) {
	s, ctx := openPostgresStore(t)
	trainer := seedTrainer(t, s, ctx, "snapshot")
	cover := seedMediaAsset(t, s, ctx, "snapshot-cover", "image", true)
	video := seedMediaAsset(t, s, ctx, "snapshot-video", "video", true)
	c := baseCase(trainer, "snapshot-case")
	c.Status = CasePublished
	c.AuthorizationStatus = AuthorizationApproved
	c.CoverAssetID = cover
	created, err := s.CreateCase(ctx, CaseAggregate{Case: c, Media: []CaseMedia{{MediaAssetID: video, Role: MediaPromo, Status: CaseMediaPublished}}})
	if err != nil {
		t.Fatal(err)
	}
	queries := 0
	s.queryObserver = func() { queries++ }
	items, err := s.ListPublicCases(ctx, PublicCaseQuery{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || queries != 3 {
		t.Fatalf("items=%d queries=%d", len(items), queries)
	}

	lockTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = lockTx.ExecContext(ctx, `LOCK TABLE training_case_topics IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	type result struct {
		items []PublicCase
		err   error
	}
	done := make(chan result, 1)
	go func() {
		v, e := s.ListPublicCases(context.Background(), PublicCaseQuery{Limit: 5})
		done <- result{v, e}
	}()
	time.Sleep(150 * time.Millisecond)
	if _, err = s.db.ExecContext(ctx, `UPDATE training_case_media SET status='offline' WHERE case_id=$1`, created.Case.ID); err != nil {
		t.Fatal(err)
	}
	if err = lockTx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatal(got.err)
		}
		if len(got.items) != 1 || len(got.items[0].Media) != 1 {
			t.Fatalf("mixed public snapshot: %+v", got.items)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("public list did not finish")
	}
}
