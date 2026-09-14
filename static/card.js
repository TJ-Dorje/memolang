// Reveal the flashcard answer by flipping one class on the card. The
// stylesheet owns what that means (show the back, show the rating form,
// hide the reveal button, run the reveal animation).
//
// This used to set element.style.display directly. Inline styles outrank
// the stylesheet, so nothing about the reveal could be styled or animated
// from CSS — the transition would always lose to the inline value.
document.addEventListener('DOMContentLoaded', function () {
    var btn = document.getElementById('reveal-btn');
    var card = document.getElementById('flashcard');
    if (!btn || !card) return;
    btn.addEventListener('click', function () {
        card.classList.add('card--flipped');
    });
});
