// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0
//
// The site's few behaviours: the colour scheme, the menu on a phone, the
// search, the copy button, the recordings of the displays and the step
// animations. Every page reads in full without this script.
(function () {
  'use strict';
  var root = document.documentElement;
  var reduce = window.matchMedia && matchMedia('(prefers-reduced-motion: reduce)').matches;

  function store(key, value) {
    try {
      if (value === undefined) return localStorage.getItem(key);
      localStorage.setItem(key, value);
    } catch (e) { return null; }
    return null;
  }

  // The colour scheme: the system's, until the reader picks one.
  var scheme = document.getElementById('scheme');
  if (scheme) {
    scheme.addEventListener('click', function () {
      var dark = root.dataset.theme ? root.dataset.theme === 'dark' : !matchMedia('(prefers-color-scheme: light)').matches;
      root.dataset.theme = dark ? 'light' : 'dark';
      store('scheme', root.dataset.theme);
    });
  }

  var menu = document.getElementById('menu');
  if (menu) {
    menu.addEventListener('click', function () {
      var open = document.body.classList.toggle('menu-open');
      menu.setAttribute('aria-expanded', open ? 'true' : 'false');
    });
  }

  document.querySelectorAll('[data-copy]').forEach(function (b) {
    b.addEventListener('click', function () {
      var text = b.getAttribute('data-copy');
      var done = function () { b.textContent = 'copied'; setTimeout(function () { b.textContent = 'copy'; }, 1500); };
      if (navigator.clipboard) navigator.clipboard.writeText(text).then(done, function () {});
    });
  });

  // Search over the titles, summaries and headings of the pages.
  var dlg = document.getElementById('search');
  var input = document.getElementById('search-input');
  var list = document.getElementById('search-list');
  var index = null, sel = 0;
  function openSearch() {
    if (!dlg || !dlg.showModal) return;
    dlg.showModal();
    input.value = '';
    render();
    if (!index) {
      fetch(dlg.getAttribute('data-index')).then(function (r) { return r.json(); }).then(function (d) { index = d; render(); });
    }
  }
  function render() {
    if (!index) { list.innerHTML = '<li class="empty">Loading the index…</li>'; return; }
    var q = input.value.trim().toLowerCase();
    var hits = index.filter(function (p) { return !q || (p.title + ' ' + p.text).toLowerCase().indexOf(q) >= 0; })
      .sort(function (a, b) { return (b.title.toLowerCase().indexOf(q) >= 0) - (a.title.toLowerCase().indexOf(q) >= 0); })
      .slice(0, 12);
    sel = 0;
    if (!hits.length) { list.innerHTML = '<li class="empty">No page has these words.</li>'; return; }
    list.innerHTML = hits.map(function (p, i) {
      var t = document.createElement('span'); t.textContent = p.title;
      var s = document.createElement('span'); s.textContent = p.section + ' · ' + p.summary;
      return '<li><a href="' + encodeURI(p.url) + '"' + (i === 0 ? ' class="sel"' : '') + '>' + t.innerHTML + '<small>' + s.innerHTML + '</small></a></li>';
    }).join('');
  }
  if (dlg) {
    document.querySelectorAll('[data-search]').forEach(function (b) { b.addEventListener('click', openSearch); });
    input.addEventListener('input', render);
    input.addEventListener('keydown', function (e) {
      var links = list.querySelectorAll('a');
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault();
        if (!links.length) return;
        links[sel].classList.remove('sel');
        sel = (sel + (e.key === 'ArrowDown' ? 1 : links.length - 1)) % links.length;
        links[sel].classList.add('sel');
      } else if (e.key === 'Enter' && links[sel]) {
        location.href = links[sel].href;
      }
    });
    dlg.addEventListener('click', function (e) { if (e.target === dlg) dlg.close(); });
    document.addEventListener('keydown', function (e) {
      var t = e.target.tagName;
      if (e.key === '/' && t !== 'INPUT' && t !== 'TEXTAREA' && t !== 'SELECT') { e.preventDefault(); openSearch(); }
    });
  }

  // The recordings: frames of the real displays, made by
  // internal/docsite on a fake clock, with the attributes of their text
  // marked «…» as progresstest.Screen.Styled marks them.
  function xterm(n) {
    if (n < 16) {
      return ['#000000', '#cd0000', '#00cd00', '#cdcd00', '#0000ee', '#cd00cd', '#00cdcd', '#e5e5e5',
        '#7f7f7f', '#ff0000', '#00ff00', '#ffff00', '#5c5cff', '#ff00ff', '#00ffff', '#ffffff'][n];
    }
    if (n >= 232) { var g = 8 + 10 * (n - 232); return 'rgb(' + g + ',' + g + ',' + g + ')'; }
    n -= 16;
    var lv = [0, 95, 135, 175, 215, 255];
    return 'rgb(' + lv[Math.floor(n / 36)] + ',' + lv[Math.floor(n / 6) % 6] + ',' + lv[n % 6] + ')';
  }
  function styled(text) {
    var frag = document.createDocumentFragment(), re = /«([^»]*)»/g, last = 0, m, span = null;
    function put(s) {
      if (!s) return;
      (span || frag).appendChild(document.createTextNode(s));
    }
    while ((m = re.exec(text))) {
      put(text.slice(last, m.index));
      if (span) { frag.appendChild(span); span = null; }
      if (m[1]) {
        span = document.createElement('span');
        var p = m[1].split(';');
        for (var i = 0; i < p.length; i++) {
          if (p[i] === '1') span.style.fontWeight = '700';
          else if (p[i] === '2') span.style.opacity = '.6';
          else if (p[i] === '4') span.style.textDecoration = 'underline';
          else if (p[i] === '38' && p[i + 1] === '5') { span.style.color = xterm(+p[i + 2]); i += 2; }
          else if (/^3[0-7]$/.test(p[i])) span.style.color = xterm(+p[i] - 30);
          else if (/^9[0-7]$/.test(p[i])) span.style.color = xterm(+p[i] - 82);
        }
      }
      last = re.lastIndex;
    }
    put(text.slice(last));
    if (span) frag.appendChild(span);
    return frag;
  }
  function visible(el, fn) {
    if (!('IntersectionObserver' in window)) { fn(true); return; }
    new IntersectionObserver(function (es) { es.forEach(function (e) { fn(e.isIntersecting); }); }).observe(el);
  }

  document.querySelectorAll('figure[data-cast]').forEach(function (fig) {
    var pre = fig.querySelector('pre'), btn = fig.querySelector('[data-act=play]'), pick = fig.querySelector('select');
    var base = fig.getAttribute('data-base'), display = fig.getAttribute('data-cast');
    var still = fig.getAttribute('data-still');
    var rec = null, i = 0, timer = null, playing = !reduce, seen = false;
    function show() {
      pre.textContent = '';
      pre.appendChild(styled(rec.frames[i].text));
    }
    function step() {
      i = (i + 1) % rec.frames.length;
      show();
      var f = rec.frames, dt = i + 1 < f.length ? f[i + 1].t - f[i].t : 3;
      timer = setTimeout(step, Math.max(120, dt * 650));
    }
    function run() {
      clearTimeout(timer);
      if (playing && seen && rec) timer = setTimeout(step, 400);
      if (btn) btn.textContent = playing ? 'pause' : 'play';
    }
    function load(theme) {
      fetch(base + display + '-' + theme + '.json').then(function (r) { return r.json(); }).then(function (d) {
        rec = d;
        if (i >= rec.frames.length || i === 0) i = still === 'last' ? rec.frames.length - 1 : Math.min(+still || 0, rec.frames.length - 1);
        show();
        run();
      });
    }
    if (btn) btn.addEventListener('click', function () { playing = !playing; run(); });
    if (pick) pick.addEventListener('change', function () { load(pick.value); });
    visible(fig, function (on) { seen = on; run(); });
    load(fig.getAttribute('data-theme'));
  });

  // Step animations: an element with data-step="n" shows from step n on,
  // one with data-only="n" at step n alone, and a mover with
  // data-at="x,y|x,y|…" stands at the place of each step.
  document.querySelectorAll('[data-steps]').forEach(function (box) {
    var caps = box.querySelectorAll('.captions li'), n = caps.length, cur = n - 1;
    var cap = box.querySelector('.ctl span'), btn = box.querySelector('[data-act=play]');
    var timer = null, playing = !reduce, seen = false;
    function show() {
      box.querySelectorAll('[data-step]').forEach(function (el) { el.style.opacity = +el.getAttribute('data-step') <= cur + 1 ? 1 : 0; });
      box.querySelectorAll('[data-only]').forEach(function (el) { el.style.opacity = +el.getAttribute('data-only') === cur + 1 ? 1 : 0; });
      box.querySelectorAll('[data-at]').forEach(function (el) {
        var at = el.getAttribute('data-at').split('|'), p = (at[cur] || at[at.length - 1]).split(',');
        el.style.transform = 'translate(' + p[0] + 'px,' + p[1] + 'px)';
      });
      cap.textContent = 'Step ' + (cur + 1) + ' of ' + n + '. ' + caps[cur].textContent;
    }
    function go(d) { cur = (cur + d + n) % n; show(); }
    function run() {
      clearTimeout(timer);
      if (playing && seen) timer = setTimeout(function tick() { go(1); timer = setTimeout(tick, 3200); }, 1200);
      btn.textContent = playing ? 'pause' : 'play';
    }
    box.querySelector('[data-act=prev]').addEventListener('click', function () { playing = false; run(); go(-1); });
    box.querySelector('[data-act=next]').addEventListener('click', function () { playing = false; run(); go(1); });
    btn.addEventListener('click', function () { playing = !playing; run(); });
    if (playing) cur = 0;
    show();
    visible(box, function (on) { seen = on; run(); });
  });
})();
