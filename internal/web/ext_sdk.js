// tagent pane SDK: tagent.api(path, {method, body}) calls /api/* through the app, which holds the access cookie.
// Classic script, no modules. The pane's origin is opaque, so messages go to "*"; only the app is the parent.
(function () {
  var pending = new Map(), seq = 0;
  window.addEventListener("message", function (e) {
    var d = e.data;
    if (e.source !== window.parent || !d || d.tagent !== "reply" || !pending.has(d.id)) return;
    var p = pending.get(d.id);
    pending.delete(d.id);
    if ("error" in d) p.reject(new Error(d.error));
    else p.resolve(d.data);
  });
  window.tagent = {
    api: function (path, opts) {
      opts = opts || {};
      var id = ++seq;
      return new Promise(function (resolve, reject) {
        pending.set(id, { resolve: resolve, reject: reject });
        window.parent.postMessage({ tagent: "api", id: id, path: path, method: opts.method || "GET", body: opts.body }, "*");
      });
    },
  };
})();
