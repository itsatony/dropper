// dropper.js — client-side behavior
// Clipboard paste, drag-drop feedback, localStorage bookmarks, toast, HTMX hooks

(function () {
  "use strict";

  // --- Constants ---
  var STORAGE_KEY_BOOKMARKS = "dropper_bookmarks";
  var STORAGE_KEY_LAST_DIR = "dropper_last_dir";
  var TOAST_DISMISS_MS = 4000;
  var TOAST_FADE_MS = 300;

  // --- Toast System ---

  function showToast(message, type) {
    var container = document.getElementById("toast-container");
    if (!container) return;

    var toast = document.createElement("div");
    toast.className = "toast toast-" + (type || "info");
    toast.textContent = message;
    container.appendChild(toast);

    setTimeout(function () {
      toast.classList.add("toast-fade");
      setTimeout(function () {
        toast.remove();
      }, TOAST_FADE_MS);
    }, TOAST_DISMISS_MS);
  }

  // Expose globally for HTMX event handlers.
  window.dropperToast = showToast;

  // --- DOM Helpers ---

  function clearChildren(el) {
    while (el.firstChild) {
      el.removeChild(el.firstChild);
    }
  }

  // --- localStorage Helpers ---

  function getBookmarks() {
    try {
      var raw = localStorage.getItem(STORAGE_KEY_BOOKMARKS);
      return raw ? JSON.parse(raw) : [];
    } catch (e) {
      return [];
    }
  }

  function saveBookmarks(bookmarks) {
    try {
      localStorage.setItem(STORAGE_KEY_BOOKMARKS, JSON.stringify(bookmarks));
    } catch (e) {
      // Storage full or unavailable — ignore.
    }
  }

  function addBookmark(path) {
    var bookmarks = getBookmarks();
    if (bookmarks.indexOf(path) === -1) {
      bookmarks.push(path);
      saveBookmarks(bookmarks);
    }
    renderBookmarks();
  }

  function removeBookmark(path) {
    var bookmarks = getBookmarks().filter(function (b) {
      return b !== path;
    });
    saveBookmarks(bookmarks);
    renderBookmarks();
  }

  function renderBookmarks() {
    var list = document.getElementById("bookmarks-list");
    if (!list) return;

    var bookmarks = getBookmarks();
    clearChildren(list);

    bookmarks.forEach(function (path) {
      var li = document.createElement("li");
      li.className = "bookmark-item";

      var a = document.createElement("a");
      a.className = "bookmark-link";
      a.href = "/?path=" + encodeURIComponent(path);
      a.textContent = path === "." ? "Home" : path;
      a.setAttribute("hx-get", "/files?path=" + encodeURIComponent(path));
      a.setAttribute("hx-target", "#file-browser");
      a.setAttribute("hx-push-url", "/?path=" + encodeURIComponent(path));
      // Let htmx process this dynamically added element.
      if (window.htmx) {
        setTimeout(function () {
          window.htmx.process(a);
        }, 0);
      }

      var btn = document.createElement("button");
      btn.className = "bookmark-remove";
      btn.type = "button";
      btn.textContent = "\u00d7";
      btn.title = "Remove bookmark";
      btn.setAttribute("aria-label", "Remove bookmark for " + path);
      btn.addEventListener("click", function (e) {
        e.preventDefault();
        e.stopPropagation();
        removeBookmark(path);
      });

      li.appendChild(a);
      li.appendChild(btn);
      list.appendChild(li);
    });
  }

  function saveLastDir(path) {
    try {
      localStorage.setItem(STORAGE_KEY_LAST_DIR, path);
    } catch (e) {
      // Ignore.
    }
  }

  function getCurrentPath() {
    var params = new URLSearchParams(window.location.search);
    return params.get("path") || ".";
  }

  // --- Drop Zone Visual Feedback ---

  function initDropzone() {
    var dropzone = document.getElementById("dropzone");
    if (!dropzone) return;

    var counter = 0;

    dropzone.addEventListener("dragenter", function (e) {
      e.preventDefault();
      counter++;
      dropzone.classList.add("drag-over");
    });

    dropzone.addEventListener("dragleave", function (e) {
      e.preventDefault();
      counter--;
      if (counter <= 0) {
        counter = 0;
        dropzone.classList.remove("drag-over");
      }
    });

    dropzone.addEventListener("dragover", function (e) {
      e.preventDefault();
    });

    dropzone.addEventListener("drop", function (e) {
      e.preventDefault();
      counter = 0;
      dropzone.classList.remove("drag-over");
      // File upload wiring deferred to Cycle 7.
      if (e.dataTransfer && e.dataTransfer.files.length > 0) {
        showToast(
          e.dataTransfer.files.length + " file(s) selected (upload wiring in next cycle)",
          "info"
        );
      }
    });
  }

  // --- Clipboard Paste Handler ---

  function initClipboardPaste() {
    document.addEventListener("paste", function (e) {
      // Only handle paste when not in an input/textarea.
      var tag = (e.target.tagName || "").toLowerCase();
      if (tag === "input" || tag === "textarea") return;

      var items = e.clipboardData && e.clipboardData.items;
      if (!items) return;

      for (var i = 0; i < items.length; i++) {
        if (items[i].type.indexOf("image/") === 0) {
          e.preventDefault();
          var blob = items[i].getAsFile();
          if (!blob) return;

          var modal = document.getElementById("preview-modal");
          var img = document.getElementById("preview-image");
          if (!modal || !img) return;

          var url = URL.createObjectURL(blob);
          img.src = url;
          modal.hidden = false;

          // Store blob for upload (Cycle 7 wiring).
          window._dropperClipboardBlob = blob;
          break;
        }
      }
    });

    // Preview modal actions.
    var confirmBtn = document.getElementById("preview-confirm");
    var cancelBtn = document.getElementById("preview-cancel");
    var backdrop = document.querySelector(".preview-backdrop");

    function closePreview() {
      var modal = document.getElementById("preview-modal");
      if (modal) modal.hidden = true;
      window._dropperClipboardBlob = null;
    }

    if (confirmBtn) {
      confirmBtn.addEventListener("click", function () {
        // Upload wiring deferred to Cycle 7.
        showToast("Clipboard upload wiring in next cycle", "info");
        closePreview();
      });
    }
    if (cancelBtn) {
      cancelBtn.addEventListener("click", closePreview);
    }
    if (backdrop) {
      backdrop.addEventListener("click", closePreview);
    }
  }

  // --- Bookmark Add Button ---

  function initBookmarkAdd() {
    var btn = document.getElementById("bookmark-add");
    if (!btn) return;

    btn.addEventListener("click", function () {
      var path = getCurrentPath();
      addBookmark(path);
      showToast("Bookmarked: " + (path === "." ? "Home" : path), "success");
    });
  }

  // --- HTMX Event Hooks ---

  function initHTMXHooks() {
    document.body.addEventListener("htmx:afterSwap", function () {
      // Re-initialize dropzone after HTMX swaps content.
      initDropzone();
      // Save current directory.
      saveLastDir(getCurrentPath());
    });

    document.body.addEventListener("htmx:pushedIntoHistory", function () {
      saveLastDir(getCurrentPath());
    });
  }

  // --- Init ---

  function init() {
    renderBookmarks();
    initDropzone();
    initClipboardPaste();
    initBookmarkAdd();
    initHTMXHooks();
    saveLastDir(getCurrentPath());
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
