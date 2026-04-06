// dropper.js — client-side behavior
// Clipboard paste, drag-drop feedback, localStorage bookmarks, toast, HTMX hooks

(function () {
  "use strict";

  // --- Constants ---
  var STORAGE_KEY_BOOKMARKS = "dropper_bookmarks";
  var STORAGE_KEY_LAST_DIR = "dropper_last_dir";
  var TOAST_DISMISS_MS = 4000;
  var TOAST_FADE_MS = 300;

  // --- Module-scoped state ---
  var clipboardBlob = null;
  var clipboardObjectURL = null;

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

  // --- File Upload ---

  function uploadFiles(files, isClipboard) {
    if (!files || files.length === 0) return;

    var formData = new FormData();
    for (var i = 0; i < files.length; i++) {
      formData.append("file", files[i]);
    }

    var url =
      "/files/upload?path=" + encodeURIComponent(getCurrentPath());
    if (isClipboard) url += "&clipboard=true";

    fetch(url, { method: "POST", body: formData })
      .then(function (resp) {
        return resp.json().then(function (data) {
          return { status: resp.status, data: data };
        });
      })
      .then(function (result) {
        if (result.status !== 200) {
          showToast(result.data.message || "Upload failed", "error");
          return;
        }
        var data = result.data;
        if (data.failed > 0) {
          showToast(data.failed + " file(s) failed to upload", "error");
        }
        if (data.uploaded > 0) {
          showToast(data.uploaded + " file(s) uploaded", "success");
        }
        refreshFileList();
      })
      .catch(function (err) {
        showToast("Upload failed: " + err.message, "error");
      });
  }

  function refreshFileList() {
    if (window.htmx) {
      var path = getCurrentPath();
      htmx.ajax("GET", "/files?path=" + encodeURIComponent(path), {
        target: "#file-browser",
      });
    }
  }

  // Expose globally for potential external use.
  window.dropperRefresh = refreshFileList;

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
      if (e.dataTransfer && e.dataTransfer.files.length > 0) {
        uploadFiles(e.dataTransfer.files, false);
      }
    });

    // Wire up the fallback file input.
    var fileInput = document.getElementById("file-input");
    if (fileInput) {
      fileInput.addEventListener("change", function () {
        if (fileInput.files.length > 0) {
          uploadFiles(fileInput.files, false);
          fileInput.value = "";
        }
      });
    }
  }

  // --- Clipboard Paste Handler ---

  function cleanupClipboardState() {
    if (clipboardObjectURL) {
      URL.revokeObjectURL(clipboardObjectURL);
      clipboardObjectURL = null;
    }
    clipboardBlob = null;
  }

  function closePreview() {
    var modal = document.getElementById("preview-modal");
    if (modal) modal.hidden = true;
    cleanupClipboardState();
  }

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

          // Clean up previous paste state before assigning new one.
          cleanupClipboardState();

          clipboardBlob = blob;
          clipboardObjectURL = URL.createObjectURL(blob);
          img.src = clipboardObjectURL;
          modal.hidden = false;
          break;
        }
      }
    });

    // Preview modal actions.
    var confirmBtn = document.getElementById("preview-confirm");
    var cancelBtn = document.getElementById("preview-cancel");
    var backdrop = document.querySelector(".preview-backdrop");

    if (confirmBtn) {
      confirmBtn.addEventListener("click", function () {
        if (clipboardBlob) {
          uploadFiles([clipboardBlob], true);
        }
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

  // --- New Folder Button ---

  function initMkdirButton() {
    var btn = document.getElementById("mkdir-btn");
    if (!btn) return;

    btn.addEventListener("click", function () {
      var name = prompt("Folder name:");
      if (!name || !name.trim()) return;

      var url =
        "/files/mkdir?path=" +
        encodeURIComponent(getCurrentPath()) +
        "&name=" +
        encodeURIComponent(name.trim());

      fetch(url, { method: "POST" })
        .then(function (resp) {
          return resp.json().then(function (data) {
            return { status: resp.status, data: data };
          });
        })
        .then(function (result) {
          if (result.status === 201) {
            showToast("Created: " + result.data.name, "success");
            refreshFileList();
          } else {
            showToast(result.data.message || "Failed to create folder", "error");
          }
        })
        .catch(function (err) {
          showToast("Failed: " + err.message, "error");
        });
    });
  }

  // --- HTMX Event Hooks ---

  function initHTMXHooks() {
    document.body.addEventListener("htmx:afterSwap", function () {
      // Re-initialize components after HTMX swaps content.
      initDropzone();
      initMkdirButton();
      renderBookmarks();
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
    initMkdirButton();
    initHTMXHooks();
    saveLastDir(getCurrentPath());
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
