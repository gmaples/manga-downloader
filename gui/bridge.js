// Copyright (C) 2023-2026 Òscar Casajuana Alonso
//
// The Android app's stand-in for the Wails runtime (see bridge.go). It gives
// the page window.go.main.App, whose methods POST to /call, and the parts of
// window.runtime the frontend uses. window.MDAndroid, when the Android app
// adds it, does what needs the system: opening links and folders, the folder
// picker and the clipboard. Without it (a desktop browser) the page's own
// APIs stand in.
(function () {
  'use strict';

  // Android 11's own WebView (Chrome 83) lacks Object.hasOwn (Chrome 93),
  // which the frontend uses; build-android.sh lowers the syntax, not the APIs
  if (typeof Object.hasOwn !== 'function') {
    Object.defineProperty(Object, 'hasOwn', {
      value: function (obj, key) {
        return Object.prototype.hasOwnProperty.call(Object(obj), key);
      },
      configurable: true,
      writable: true,
    });
  }

  var token = new URLSearchParams(window.location.search).get('t') || '';
  var native = function () {
    return window.MDAndroid;
  };

  // Events: one stream for the page, fanned out by name
  var listeners = {};
  var source = null;
  function dispatch(name, data) {
    var list = listeners[name];
    if (!list) {
      return;
    }
    list.slice().forEach(function (l) {
      if (l.left === 0) {
        return;
      }
      if (l.left > 0) {
        l.left--;
      }
      try {
        l.cb(data);
      } catch (err) {
        console.error('event ' + name + ':', err);
      }
    });
    listeners[name] = list.filter(function (l) {
      return l.left !== 0;
    });
  }
  function connect() {
    if (source !== null) {
      return;
    }
    source = new EventSource('/events?t=' + encodeURIComponent(token));
    source.onmessage = function (e) {
      var msg;
      try {
        msg = JSON.parse(e.data);
      } catch (err) {
        console.error('bad event:', err);
        return;
      }
      dispatch(msg.event, msg.data);
    };
    // EventSource reconnects on its own after an error
  }

  function call(method, args) {
    return fetch('/call/' + method, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-MD-Token': token },
      body: JSON.stringify(args),
    }).then(function (res) {
      if (!res.ok) {
        return res.text().then(function (text) {
          throw text || res.statusText;
        });
      }
      return res.json().then(function (out) {
        // rejected with the string the window's ErrorFormatter makes, as Wails does
        if (out.error !== undefined) {
          throw out.error;
        }
        return out.result;
      });
    });
  }

  var app = new Proxy(
    {},
    {
      get: function (_, method) {
        if (typeof method !== 'string') {
          return undefined;
        }
        return function () {
          return call(method, Array.prototype.slice.call(arguments));
        };
      },
    },
  );
  window.go = { main: { App: app } };

  window.runtime = {
    EventsOnMultiple: function (name, cb, max) {
      connect();
      var l = { cb: cb, left: max === undefined ? -1 : max };
      (listeners[name] = listeners[name] || []).push(l);
      return function () {
        l.left = 0;
        listeners[name] = (listeners[name] || []).filter(function (x) {
          return x !== l;
        });
      };
    },
    EventsOff: function () {
      for (var i = 0; i < arguments.length; i++) {
        delete listeners[arguments[i]];
      }
    },
    EventsOffAll: function () {
      listeners = {};
    },
    BrowserOpenURL: function (url) {
      if (native()) {
        native().openURL(String(url));
      } else {
        window.open(String(url), '_blank', 'noopener');
      }
    },
    ClipboardGetText: function () {
      if (native()) {
        return Promise.resolve(native().clipboardText());
      }
      return navigator.clipboard.readText();
    },
  };

  // The Go side's folder picker (ChooseDirectory) and OpenFolder ask the page.
  // MDAndroid.pickFolder answers through window.__mdFolderPicked.
  function answerPick(id, path) {
    fetch('/picked?id=' + encodeURIComponent(id), {
      method: 'POST',
      headers: { 'X-MD-Token': token },
      body: path || '',
    }).catch(function (err) {
      console.error('could not answer the folder picker:', err);
    });
  }
  window.__mdFolderPicked = answerPick;
  window.runtime.EventsOnMultiple(
    'bridge:chooseDirectory',
    function (req) {
      if (native()) {
        native().pickFolder(req.id);
      } else {
        answerPick(req.id, '');
      }
    },
    -1,
  );
  window.runtime.EventsOnMultiple(
    'bridge:openFolder',
    function (req) {
      if (native()) {
        native().openFolder(String(req.path));
      }
    },
    -1,
  );
})();
