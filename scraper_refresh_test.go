package main

import "testing"

func TestHasNewPostComparedToCache_NoBaseline(t *testing.T) {
	latest := []PostItem{{Index: 1, Shortcode: "new1"}}
	if hasNewPostComparedToCache(nil, latest) {
		t.Fatal("expected false when no cached baseline")
	}
}

func TestHasNewPostComparedToCache_ExpiredButUnchanged(t *testing.T) {
	cached := []PostItem{
		{Index: 1, Shortcode: "A"},
		{Index: 2, Shortcode: "B"},
		{Index: 3, Shortcode: "C"},
	}
	latest := []PostItem{
		{Index: 1, Shortcode: "A"},
		{Index: 2, Shortcode: "B"},
		{Index: 3, Shortcode: "C"},
	}
	if hasNewPostComparedToCache(cached, latest) {
		t.Fatal("expected false for unchanged posts")
	}
}

func TestHasNewPostComparedToCache_NewPostAppears(t *testing.T) {
	cached := []PostItem{
		{Index: 1, Shortcode: "A"},
		{Index: 2, Shortcode: "B"},
		{Index: 3, Shortcode: "C"},
	}
	latest := []PostItem{
		{Index: 1, Shortcode: "X"},
		{Index: 2, Shortcode: "A"},
		{Index: 3, Shortcode: "B"},
	}
	if !hasNewPostComparedToCache(cached, latest) {
		t.Fatal("expected true when unseen shortcode appears in latest range")
	}
}

func TestHasNewPostComparedToCache_ReorderOnly(t *testing.T) {
	cached := []PostItem{
		{Index: 1, Shortcode: "A"},
		{Index: 2, Shortcode: "B"},
		{Index: 3, Shortcode: "C"},
	}
	latest := []PostItem{
		{Index: 1, Shortcode: "B"},
		{Index: 2, Shortcode: "A"},
		{Index: 3, Shortcode: "C"},
	}
	if hasNewPostComparedToCache(cached, latest) {
		t.Fatal("expected false for reorder-only change")
	}
}

func TestSamePostsOrder(t *testing.T) {
	a := []PostItem{{Index: 1, Shortcode: "A"}, {Index: 2, Shortcode: "B"}}
	b := []PostItem{{Index: 1, Shortcode: "A"}, {Index: 2, Shortcode: "B"}}
	c := []PostItem{{Index: 1, Shortcode: "B"}, {Index: 2, Shortcode: "A"}}

	if !samePostsOrder(a, b) {
		t.Fatal("expected true for same order")
	}
	if samePostsOrder(a, c) {
		t.Fatal("expected false for different order")
	}
}
