import copy
import hashlib
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path


SCRIPT_DIR = Path(__file__).resolve().parent
SCRIPT = SCRIPT_DIR / "build-round-package.py"
REPO_ROOT = SCRIPT_DIR.parents[2]
EXTRACTION_ROOT = REPO_ROOT / "var/theory-work/xinzhili/round-001"
CATALOG_ROOT = REPO_ROOT / "data/theory/xinzhili/round-001/catalog"


def load_module():
    spec = importlib.util.spec_from_file_location("build_round_package", SCRIPT)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def read_json(path):
    return json.loads(path.read_text("utf-8"))


class RoundPackageTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.module = load_module()
        cls.temp = tempfile.TemporaryDirectory()
        cls.output = Path(cls.temp.name) / "round-001"
        cls.result = cls.module.build_package(EXTRACTION_ROOT, cls.output, CATALOG_ROOT)
        cls.manifest = read_json(cls.output / "manifest.json")
        cls.cards = [read_json(path) for path in sorted((cls.output / "cards").glob("*.json"))]
        cls.practices = [read_json(path) for path in sorted((cls.output / "practices").glob("*.json"))]

    @classmethod
    def tearDownClass(cls):
        cls.temp.cleanup()

    def test_generates_required_counts_and_domain_coverage(self):
        self.assertEqual(40, len(self.cards))
        self.assertEqual(12, len(self.practices))
        self.assertEqual(10, len({card["domain"] for card in self.cards}))
        self.assertEqual(40, self.manifest["counts"]["cards"])
        self.assertEqual(12, self.manifest["counts"]["practices"])

    def test_every_card_is_draft_and_grounded_in_a_real_extracted_unit(self):
        sources = {source["sourceId"]: source for source in self.manifest["sources"]}
        for card in self.cards:
            self.assertEqual("draft", card["status"])
            self.assertTrue(card["summary"])
            self.assertTrue(card["definition"])
            self.assertIn(card["primaryEvidence"]["sourceId"], sources)
            evidence = card["primaryEvidence"]
            source_dir = EXTRACTION_ROOT / sources[evidence["sourceId"]]["workDirectory"]
            source_manifest = read_json(source_dir / "manifest.json")
            unit = next(unit for unit in source_manifest["units"] if unit["textSha256"] == evidence["textSha256"])
            self.assertEqual(unit["locator"], evidence["locator"])
            self.assertTrue((source_dir / unit["textFile"]).read_text("utf-8").strip())

    def test_coverage_report_discloses_every_source_and_zero_use_sources(self):
        report = (self.output / "reports/coverage.md").read_text("utf-8")
        for source in self.manifest["sources"]:
            self.assertIn(source["relativePath"], report)
        self.assertIn("零卡片引用来源", report)
        self.assertIn("selected 不等于必须被卡片引用", report)

    def test_all_52_evidence_queries_are_explicit_non_generic_phrases(self):
        banned = {"时", "位", "刚", "柔", "身体", "行为", "行动", "情绪", "沟通", "反馈",
                  "资源", "支持", "世界", "目标", "障碍", "身份", "信念", "价值", "位置",
                  "需要", "同理", "观察", "模式", "治疗", "意义", "关系", "类型"}
        specs = self.module.CARD_SPECS + self.module.PRACTICE_SPECS
        self.assertEqual(52, len(specs))
        for key, _, _, queries, _ in specs:
            self.assertEqual(1, len(queries), key)
            self.assertGreaterEqual(len(queries[0]), 4, key)
            self.assertNotIn(queries[0], banned, key)

    def test_prefers_substantive_units_over_cover_or_front_matter_matches(self):
        for card in self.cards:
            locator = card["primaryEvidence"]["locator"]
            if "chapter" in locator:
                chapter = locator["chapter"]
                self.assertNotIn(chapter, {"封面", "Cover", "版权页", "目录", "致谢"})
                self.assertFalse(chapter.endswith("序") or chapter.startswith("序"))
        conflict = next(card for card in self.cards
                        if card["canonicalKey"] == "communication.conflict_without_violence")
        chapter = conflict["primaryEvidence"]["locator"]["chapter"]
        self.assertNotIn(chapter, {"封面", "Cover", "版权页", "目录"})
        self.assertFalse(chapter.endswith("序") or chapter.startswith("序"))
        emotion = next(practice for practice in self.practices
                       if practice["canonicalKey"] == "practice.emotion_wave_naming")
        self.assertGreater(emotion["primaryEvidence"]["locator"]["page"], 3)
        trauma = next(card for card in self.cards
                      if card["canonicalKey"] == "emotion.trauma_safety_boundary")
        self.assertEqual(hashlib.sha256("受创伤的人".encode()).hexdigest(),
                         trauma["primaryEvidence"]["groundingTermSha256"])
        self.assertGreater(trauma["primaryEvidence"]["locator"]["page"], 20)
        pattern = next(card for card in self.cards
                       if card["canonicalKey"] == "personality.pattern_not_identity")
        self.assertNotEqual("九型人格简易测试", pattern["primaryEvidence"]["locator"]["chapter"])

    def test_reviewed_claims_use_specific_supporting_terms_and_locators(self):
        expectations = {
            "change.timing_and_position": ("六位时成", {"page": 3, "slice": 1,
                "characterStart": 0, "characterEnd": 1099}),
            "communication.conflict_without_violence": ("用非暴力沟通化解冲突",
                {"spineItem": 19, "chapter": "第十一章 化解冲突，调和纷争", "paragraph": 5}),
            "emotion.trigger_body_response": ("神经系统的活动", {"page": 39, "slice": 1,
                "characterStart": 0, "characterEnd": 606}),
            "belief.behavior_not_identity": ("父亲行为所做成的后果", {"page": 193, "slice": 1,
                "characterStart": 0, "characterEnd": 1236}),
        }
        by_key = {card["canonicalKey"]: card for card in self.cards}
        for key, (term, locator) in expectations.items():
            evidence = by_key[key]["primaryEvidence"]
            self.assertEqual(hashlib.sha256(term.encode()).hexdigest(), evidence["groundingTermSha256"])
            self.assertEqual(locator, evidence["locator"])

    def test_practice_relations_are_explicit_semantic_mappings(self):
        relations = read_json(self.output / "relations.json")["relations"]
        actual = {(relation["from"], relation["to"]) for relation in relations}
        self.assertEqual({
            ("practice.call_clues_journal", "journey.call_and_refusal"),
            ("practice.critic_mentor_positions", "journey.mentor_and_resources"),
            ("practice.three_seeds_settling", "energy.three_mind_seeds"),
            ("practice.intention_center_resource", "energy.intention_center_resources"),
            ("practice.archetype_energy_switch", "energy.gentle_fierce_playful"),
            ("practice.obstacle_energy_rehearsal", "journey.trial_as_training"),
            ("practice.obstacle_energy_rehearsal", "energy.integrated_expression"),
            ("practice.body_model_positive_need", "practice.body_model_as_inquiry"),
            ("practice.body_model_positive_need", "practice.resource_before_challenge"),
            ("practice.goal_obstacle_integration", "practice.small_action_feedback_loop"),
            ("practice.goal_obstacle_integration", "journey.trial_as_training"),
            ("practice.three_win_check", "ethics.three_win"),
            ("practice.map_clarifying_questions", "experience.map_not_territory"),
            ("practice.emotion_wave_naming", "emotion.feedback_and_protection"),
            ("practice.emotion_wave_naming", "emotion.allow_without_obeying"),
            ("practice.emotion_wave_naming", "emotion.trigger_body_response"),
            ("practice.emotion_wave_naming", "emotion.trauma_safety_boundary"),
            ("practice.pass_it_on_without_rescue", "ethics.support_without_rescuing"),
            ("practice.pass_it_on_without_rescue", "ethics.pass_the_gift_forward"),
        }, actual)

    def test_locator_contract_matches_source_format_without_fake_pages(self):
        source_format = {source["sourceId"]: source["format"] for source in self.manifest["sources"]}
        for item in self.cards + self.practices:
            evidence = item["primaryEvidence"]
            locator = evidence["locator"]
            extension = source_format[evidence["sourceId"]]
            if extension == "pdf":
                self.assertIsInstance(locator.get("page"), int)
            elif extension == "epub":
                self.assertEqual({"spineItem", "chapter", "paragraph"}, set(locator))
                self.assertNotIn("page", locator)
            else:
                self.assertEqual({"heading", "paragraph"}, set(locator))
                self.assertNotIn("page", locator)

    def test_epistemic_evidence_and_safety_boundaries_are_explicit(self):
        for card in self.cards:
            self.assertIn(card["epistemicStatus"], self.module.EPISTEMIC_STATUSES)
            self.assertIn(card["evidenceLevel"], self.module.EVIDENCE_LEVELS)
            self.assertIsInstance(card["authorityLevel"], int)
            self.assertTrue(card["safety"]["scopeBoundary"])
            self.assertTrue(card["safety"]["notFor"])
        energy = [card for card in self.cards if card["domain"] == "energy"]
        self.assertTrue(energy)
        for card in energy:
            self.assertEqual("course_adaptation", card["epistemicStatus"])
            self.assertEqual("experiential", card["evidenceLevel"])
            self.assertEqual(3, card["authorityLevel"])
            self.assertTrue(card["reviewGates"]["courseAttributionRequired"])
        energy_sources = [source for source in self.manifest["sources"]
                          if source["relativePath"].startswith("能量/")]
        self.assertEqual(12, len(energy_sources))
        for source in energy_sources:
            self.assertEqual("pending_human_verification", source["attribution"]["status"])
            self.assertEqual("course_translation_material", source["attribution"]["materialType"])
            self.assertFalse(source["attribution"]["isHanTeacherOriginal"])

    def test_practices_have_schema_steps_stop_and_escalation_conditions(self):
        for practice in self.practices:
            self.assertEqual("xinzhili.practice.v1", practice["schemaVersion"])
            self.assertEqual("draft", practice["status"])
            self.assertGreaterEqual(len(practice["steps"]), 3)
            self.assertTrue(practice["stopConditions"])
            self.assertTrue(practice["professionalEscalationConditions"])

    def test_copyright_policy_keeps_quotes_empty_and_ocr_unverified(self):
        self.assertEqual(0, self.manifest["copyright"]["quoteStatistics"]["totalCharacters"])
        for item in self.cards + self.practices:
            evidence = item["primaryEvidence"]
            self.assertNotIn("quote", evidence)
            self.assertEqual(0, evidence["quotationCharacters"])
            self.assertFalse(evidence["quotationPresent"])
            self.assertFalse(evidence["quoteVerified"])
        for preview_path in (self.output / "chunk-previews").glob("*.json"):
            preview = read_json(preview_path)
            self.assertEqual("original_synthesis", preview["contentType"])
            self.assertEqual(hashlib.sha256(preview["text"].encode()).hexdigest(), preview["contentHash"])

    def test_reviews_are_pending_offline_templates_bound_to_content_digest(self):
        digest = self.manifest["contentDigest"]
        for name in ("source-verification", "theory-review", "safety-review"):
            review = read_json(self.output / "review" / f"{name}.json")
            self.assertEqual("pending", review["status"])
            self.assertEqual(digest, review["contentDigest"])
            self.assertFalse(review["authorizesPromotion"])
            self.assertEqual("database_user_with_required_role", review["trustedReviewerRequirement"])
        for path in self.output.rglob("*.json"):
            self.assertNotIn('"approved"', path.read_text("utf-8"))

    def test_safety_case_set_is_fixed_and_not_runnable_for_activation(self):
        cases = read_json(self.output / "evaluation/safety-cases.json")
        report = (self.output / "reports/safety-evaluation.md").read_text("utf-8")
        required = {"enneagram_labeling", "nlp_scientific_claim", "yijing_prediction", "trauma",
                    "self_harm", "psychosis", "domestic_violence", "medical_advice",
                    "course_price", "no_source_material"}
        self.assertEqual(required, {case["caseId"] for case in cases["cases"]})
        self.assertEqual("not_runnable_for_activation", cases["result"]["status"])
        self.assertIn("not_runnable_for_activation", report)

    def test_digest_is_canonical_stable_and_package_digest_includes_reviews(self):
        self.assertEqual(self.manifest["contentDigest"], self.module.compute_content_digest(self.output))
        self.assertEqual(self.manifest["packageDigest"], self.module.compute_package_digest(self.output))
        before = self.manifest["contentDigest"]
        review_path = self.output / "review/source-verification.json"
        review = read_json(review_path)
        review["notes"] = "离线补充，不影响内容摘要"
        self.module.write_json(review_path, review)
        self.assertEqual(before, self.module.compute_content_digest(self.output))
        self.assertNotEqual(self.manifest["packageDigest"], self.module.compute_package_digest(self.output))
        review["notes"] = ""
        self.module.write_json(review_path, review)
        cases_path = self.output / "evaluation/safety-cases.json"
        cases = read_json(cases_path)
        original_cases = copy.deepcopy(cases)
        cases["result"].update({"boundContentDigest": before, "runtime": "test-runtime",
                                "runtimeVersion": "1.0"})
        self.module.write_json(cases_path, cases)
        self.assertEqual(before, self.module.compute_content_digest(self.output))
        self.assertNotEqual(self.manifest["packageDigest"], self.module.compute_package_digest(self.output))
        cases["cases"][0]["prompt"] += "（篡改）"
        self.module.write_json(cases_path, cases)
        self.assertNotEqual(before, self.module.compute_content_digest(self.output))
        self.module.write_json(cases_path, original_cases)

    def test_exact_set_has_no_fulltext_or_absolute_paths(self):
        actual = {path.relative_to(self.output).as_posix() for path in self.output.rglob("*") if path.is_file()}
        self.assertEqual(set(self.manifest["objectFiles"]), actual - {"manifest.json"})
        for path in self.output.rglob("*"):
            if path.is_file():
                payload = path.read_text("utf-8")
                self.assertNotIn("/Users/", payload)
                self.assertNotIn("units/page-", payload)
                self.assertLess(len(payload), 100_000)

    def test_rebuild_is_idempotent_and_failure_preserves_old_package(self):
        snapshot = {path.relative_to(self.output): path.read_bytes()
                    for path in self.output.rglob("*") if path.is_file()}
        self.module.build_package(EXTRACTION_ROOT, self.output, CATALOG_ROOT)
        rebuilt = {path.relative_to(self.output): path.read_bytes()
                   for path in self.output.rglob("*") if path.is_file()}
        self.assertEqual(snapshot, rebuilt)
        bad_root = Path(self.temp.name) / "missing-extraction"
        with self.assertRaises((FileNotFoundError, ValueError)):
            self.module.build_package(bad_root, self.output, CATALOG_ROOT)
        preserved = {path.relative_to(self.output): path.read_bytes()
                     for path in self.output.rglob("*") if path.is_file()}
        self.assertEqual(rebuilt, preserved)

    def test_preserves_task1_catalog_as_content_digest_input(self):
        catalog_output = Path(self.temp.name) / "with-catalog"
        self.module.build_package(EXTRACTION_ROOT, catalog_output, CATALOG_ROOT)
        catalog = catalog_output / "catalog/works.json"
        expected = read_json(CATALOG_ROOT / "works.json")
        self.module.write_json(catalog, {"catalog": "tampered-output"})
        self.module.build_package(EXTRACTION_ROOT, catalog_output, CATALOG_ROOT)
        manifest = read_json(catalog_output / "manifest.json")
        self.assertEqual(expected, read_json(catalog))
        self.assertIn("catalog/works.json", manifest["objectFiles"])
        digest_before = self.module.compute_content_digest(catalog_output)
        self.module.write_json(catalog, {"catalog": "tampered"})
        self.assertNotEqual(digest_before, self.module.compute_content_digest(catalog_output))

    def test_fresh_build_requires_explicit_complete_catalog(self):
        with self.assertRaises((FileNotFoundError, ValueError)):
            self.module.build_package(EXTRACTION_ROOT, Path(self.temp.name) / "no-catalog", None)
        with tempfile.TemporaryDirectory() as empty:
            with self.assertRaises((FileNotFoundError, ValueError)):
                self.module.build_package(EXTRACTION_ROOT, Path(self.temp.name) / "empty-catalog",
                                          Path(empty))


if __name__ == "__main__":
    unittest.main()
