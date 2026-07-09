package videoproject

import (
	"reflect"
	"strings"
	"testing"
)

func TestEnhanceActionUsesLongestDictionaryMatch(t *testing.T) {
	b := &PromptBuilder{}

	got := b.enhanceAction("女孩走进森林")

	if !strings.Contains(got, "walking into") {
		t.Fatalf("expected longest match walking into, got %q", got)
	}
	if strings.Contains(got, "walking slowly") {
		t.Fatalf("used shorter dictionary match: %q", got)
	}
}

func TestBuildReferenceImagesPriorityAndLimit(t *testing.T) {
	b := &PromptBuilder{}
	prev := &Shot{EndFrameURL: "prev.jpg"}
	scene := &Scene{ReferenceImageURL: "scene.jpg"}
	chars := []Character{
		{ReferenceImageURL: "side.jpg"},
		{ReferenceImageURL: "main.jpg", IsMain: true},
	}
	shot := Shot{ImageReferenceModes: []string{"prev_frame", "character_ref", "scene_ref"}}

	got := b.buildReferenceImages(shot, chars, scene, prev)
	want := []string{"prev.jpg", "main.jpg", "side.jpg", "scene.jpg"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestBuildReferenceImagesDeduplicatesAndCapsAtFour(t *testing.T) {
	b := &PromptBuilder{}
	prev := &Shot{EndFrameURL: "same.jpg"}
	scene := &Scene{ReferenceImageURL: "scene.jpg"}
	chars := []Character{
		{ReferenceImageURL: "same.jpg", IsMain: true},
		{ReferenceImageURL: "side-1.jpg"},
		{ReferenceImageURL: "side-2.jpg"},
		{ReferenceImageURL: "side-3.jpg"},
	}
	shot := Shot{ImageReferenceModes: []string{"prev_frame", "character_ref", "scene_ref"}}

	got := b.buildReferenceImages(shot, chars, scene, prev)
	want := []string{"same.jpg", "side-1.jpg", "side-2.jpg", "side-3.jpg"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestBuildReferenceImagesPrioritizesExplicitShotAssets(t *testing.T) {
	b := &PromptBuilder{}
	prev := &Shot{EndFrameURL: "prev.jpg"}
	scene := &Scene{ReferenceImageURL: "scene.jpg"}
	chars := []Character{
		{ReferenceImageURL: "main.jpg", IsMain: true},
		{ReferenceImageURL: "side-1.jpg"},
		{ReferenceImageURL: "side-2.jpg"},
	}
	shot := Shot{
		ImageReferenceModes: []string{"prev_frame", "character_ref", "scene_ref"},
		ShotAssets: []ShotAsset{
			{AssetType: "image", ObjectURL: "explicit-shot-reference.jpg"},
		},
	}

	got := b.buildReferenceImages(shot, chars, scene, prev)

	if !reflect.DeepEqual(got, []string{"explicit-shot-reference.jpg", "prev.jpg", "main.jpg", "side-1.jpg"}) {
		t.Fatalf("explicit shot asset should be kept ahead of automatic references, got %#v", got)
	}
}

func TestBuildReferenceVideosPrioritizesExplicitShotAssets(t *testing.T) {
	b := &PromptBuilder{}
	scene := &Scene{ReferenceVideoURL: "scene-demo.mp4"}
	shot := Shot{
		VideoReferenceMode: "scene_demo",
		ShotAssets: []ShotAsset{
			{AssetType: "video", ObjectURL: "explicit-video-1.mp4"},
			{AssetType: "video", ObjectURL: "explicit-video-2.mp4"},
		},
	}

	got := b.buildReferenceVideos(shot, scene, nil)
	want := []string{"explicit-video-1.mp4", "explicit-video-2.mp4"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("explicit shot videos should be kept ahead of automatic video references, got %#v want %#v", got, want)
	}
}
