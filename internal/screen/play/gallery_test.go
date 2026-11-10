package play

import "testing"

// TestGalleryReviewButton: the picture review opens by its button, as by Tab.
func TestGalleryReviewButton(t *testing.T) {
	g, _ := mnuReviewGame(t, true)
	s := newGallery(g)
	g.push(s)
	mnuTap(g, s.reviewButton(g))
	if _, ok := mnuTop(g).(*cgReview); !ok {
		t.Fatalf("a tap on the review button left %T on top", mnuTop(g))
	}
}
