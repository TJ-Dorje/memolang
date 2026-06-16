document.addEventListener('DOMContentLoaded', function () {
    var btn = document.getElementById('reveal-btn');
    if (!btn) return;
    btn.addEventListener('click', function () {
        document.getElementById('card-back').style.display = 'block';
        document.getElementById('answer-form').style.display = 'block';
        btn.style.display = 'none';
    });
});
