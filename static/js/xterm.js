(function(){
  // Load xterm CSS and JS from CDN dynamically
  try {
    var l = document.createElement('link');
    l.rel = 'stylesheet';
    l.href = 'https://unpkg.com/xterm@5.1.0/css/xterm.css';
    document.head.appendChild(l);

    var s = document.createElement('script');
    s.src = 'https://unpkg.com/xterm@5.1.0/lib/xterm.js';
    s.async = true;
    document.head.appendChild(s);
  } catch (e) {
    console.error('Failed to load xterm assets', e);
  }
})();
