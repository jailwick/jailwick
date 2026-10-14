// Apply the saved theme before the first paint (no flash). "system" follows the OS.
(function () {
  var pref = 'system'
  try { pref = localStorage.getItem('jailwick.theme') || 'system' } catch (e) {}
  var dark = pref === 'dark' || (pref === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.setAttribute('data-theme', dark ? 'dark' : 'light')
})()
