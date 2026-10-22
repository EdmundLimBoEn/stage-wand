(() => {
  const $ = (id) => document.getElementById(id);
  const tabs = [...document.querySelectorAll('[role="tab"]')];
  const select = (t, focus) => {
    tabs.forEach(x => {
      const on = x === t;
      x.setAttribute('aria-selected', on);
      x.tabIndex = on ? 0 : -1;
      $(x.getAttribute('aria-controls')).hidden = !on;
    });
    if (focus) t.focus();
  };
  tabs.forEach((t, i) => {
    t.addEventListener('click', () => select(t));
    t.addEventListener('keydown', e => {
      const d = { ArrowRight: 1, ArrowLeft: -1 }[e.key];
      if (d) { e.preventDefault(); select(tabs[(i + d + tabs.length) % tabs.length], true); }
    });
  });
  document.querySelectorAll('[data-tab]').forEach(a => a.addEventListener('click', () => select($('t-' + a.dataset.tab))));

  const ua = navigator.userAgent;
  const hint = (navigator.userAgentData && navigator.userAgentData.platform) || '';
  const os = /Android/i.test(hint + ua) ? 'android'
    : /iPhone|iPad|iPod/.test(ua) || (/Macintosh/.test(ua) && navigator.maxTouchPoints > 1) ? 'ios'
    : /Windows/i.test(hint + ua) ? 'win'
    : /mac/i.test(hint) || /Macintosh/.test(ua) ? 'mac'
    : /Linux|CrOS/i.test(hint + ua) ? 'linux' : null;

  const TF = 'https://testflight.apple.com/join/JHGAMYwU';
  const cta = {
    ios:     ['Join the iPhone beta', TF, 'Download for macOS', '/download/mac', 'TestFlight beta, iOS 17 or later. Pair it with a Mac, Windows or Linux host.'],
    win:     ['Windows setup', '#setup', 'Join the iPhone beta', TF, 'A prebuilt Windows download is coming soon. Build the host from source today.'],
    linux:   ['Download for Linux', '/download/linux', 'Join the iPhone beta', TF, 'For x86_64 Linux. Arch and Debian commands are in Setup below.'],
    android: ['Android setup', '#setup', 'Download for macOS', '/download/mac', 'The Android remote builds from source today; a release is coming soon.'],
  }[os];
  if (cta) {
    const [l1, h1, l2, h2, note] = cta;
    Object.assign($('cta-primary'), { textContent: l1, href: h1 });
    Object.assign($('cta-secondary'), { textContent: l2, href: h2 });
    $('cta-note').textContent = note;
    if (h1.startsWith('#')) $('cta-primary').addEventListener('click', () => select($('t-' + os)));
  }
  const hash = { '#mac': 'mac', '#windows': 'win', '#linux': 'linux', '#iphone': 'ios', '#android': 'android' }[location.hash];
  const un = new URLSearchParams(location.search).get('unavailable');
  const unTab = { windows: 'win', android: 'android' }[un];
  select($('t-' + (unTab || hash || os || 'mac')));
})();
