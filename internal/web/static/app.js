// Tell the program the window is still open; it exits when all windows close.
(function () {
  function beat() { fetch('/api/heartbeat', { method: 'POST' }).catch(function () {}); }
  beat();
  setInterval(beat, 5000);
})();

// Ask before destructive actions: <form data-confirm="...">
document.addEventListener('submit', function (e) {
  var msg = e.target.getAttribute('data-confirm');
  if (msg && !confirm(msg)) e.preventDefault();
});

// Filter a <select> by typed text: <input data-filter-select="id">
document.addEventListener('input', function (e) {
  var id = e.target.getAttribute('data-filter-select');
  if (!id) return;
  var sel = document.getElementById(id);
  var q = e.target.value.toLowerCase();
  var first = null;
  for (var i = 0; i < sel.options.length; i++) {
    var o = sel.options[i];
    if (!o.value) continue;
    var show = o.text.toLowerCase().indexOf(q) >= 0;
    o.hidden = !show;
    if (show && !first) first = o;
  }
  if (q && first && (sel.selectedOptions.length === 0 || sel.selectedOptions[0].hidden || !sel.value)) {
    sel.value = first.value;
    sel.dispatchEvent(new Event('change', { bubbles: true }));
  }
});

// Show/hide parts of a form depending on a radio: data-show-when="name=value"
function updateShowWhen() {
  document.querySelectorAll('[data-show-when]').forEach(function (el) {
    var parts = el.getAttribute('data-show-when').split('=');
    var checked = document.querySelector('input[name="' + parts[0] + '"]:checked');
    el.hidden = !(checked && checked.value === parts[1]);
  });
}
document.addEventListener('change', updateShowWhen);
document.addEventListener('DOMContentLoaded', updateShowWhen);
