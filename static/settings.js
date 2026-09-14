// Progressive enhancement for the provider picker: when the choice changes,
// refill Base URL and Model from that provider's preset and update the note
// and key link.
//
// Nothing here is required for the form to work. With JavaScript off the
// fields keep whatever the server rendered, and SaveSettings falls back to the
// same presets — so the only thing lost is seeing the values update before
// saving. That is why the presets live in Go and are handed over on data-*
// attributes rather than being duplicated in this file.
document.addEventListener('DOMContentLoaded', function () {
    var select = document.getElementById('provider');
    if (!select) return;

    var baseURL = document.getElementById('base_url');
    var model = document.getElementById('model');
    var note = document.getElementById('provider-note');

    // Remember what the user had stored, so switching away from their provider
    // and back does not discard a customised URL or model.
    var stored = {
        provider: select.value,
        baseURL: baseURL ? baseURL.value : '',
        model: model ? model.value : ''
    };

    function apply() {
        var opt = select.options[select.selectedIndex];
        if (!opt) return;

        var isStored = select.value === stored.provider;
        if (baseURL) {
            baseURL.value = isStored ? stored.baseURL : opt.dataset.baseUrl || '';
        }
        if (model) {
            model.value = isStored ? stored.model : opt.dataset.model || '';
        }
        if (note) {
            note.textContent = opt.dataset.note || '';
        }

        updateKeyLink(opt);
    }

    function updateKeyLink(opt) {
        var row = document.getElementById('key-link-row');
        var url = opt.dataset.keyUrl || '';

        if (!url) {
            if (row) row.hidden = true;
            return;
        }

        if (!row) {
            row = document.createElement('div');
            row.id = 'key-link-row';
            row.innerHTML = '<small class="hint">Need a key? <a target="_blank" rel="noopener noreferrer">Get one here →</a></small>';
            select.parentNode.insertAdjacentElement('afterend', row);
        }
        row.hidden = false;
        row.querySelector('a').href = url;
    }

    select.addEventListener('change', apply);
});
