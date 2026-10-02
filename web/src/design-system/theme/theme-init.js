(function () {
  var preference = 'system';
  try {
    var stored = window.localStorage.getItem('aib.theme');
    if (stored === 'light' || stored === 'dark' || stored === 'system') preference = stored;
  } catch {
    // Blocked storage uses the system appearance on first paint.
  }
  var theme = preference === 'system'
    ? (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
    : preference;
  document.documentElement.dataset.theme = theme;
  document.documentElement.style.colorScheme = theme;
})();
