// dropper.js — client-side behavior
// Clipboard paste, drag-drop (flat + directory), localStorage bookmarks,
// toast, upload progress, last-dir navigation, HTMX hooks

(function () {
  "use strict";

  // --- Constants ---
  var STORAGE_KEY_BOOKMARKS = "dropper_bookmarks";
  var STORAGE_KEY_LAST_DIR = "dropper_last_dir";
  var TOAST_DISMISS_MS = 4000;
  var TOAST_FADE_MS = 300;
  var PROGRESS_HIDE_DELAY_MS = 500;

  // --- User-facing messages ---
  var MSG_UPLOAD_FAILED = "Upload failed";
  var MSG_UPLOAD_INVALID_RESPONSE = "Upload failed: invalid response";
  var MSG_UPLOAD_NETWORK_ERROR = "Upload failed: network error";
  var MSG_UPLOAD_CANCELLED = "Upload cancelled";
  var MSG_UPLOAD_FILES_FAILED = " file(s) failed to upload";
  var MSG_UPLOAD_FILES_SUCCESS = " file(s) uploaded";
  var MSG_MKDIR_FAILED = "Failed to create folder";
  var MSG_MKDIR_PREFIX = "Created: ";
  var MSG_MKDIR_ERROR_PREFIX = "Failed: ";
  var MSG_BOOKMARK_PREFIX = "Bookmarked: ";
  var MSG_BOOKMARK_HOME = "Home";

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

  function getLastDir() {
    try {
      return localStorage.getItem(STORAGE_KEY_LAST_DIR);
    } catch (e) {
      return null;
    }
  }

  function getCurrentPath() {
    var params = new URLSearchParams(window.location.search);
    return params.get("path") || ".";
  }

  // --- Route URLs (read from data attributes set by templates) ---

  function getUploadURL() {
    var el = document.getElementById("dropzone");
    return (el && el.getAttribute("data-upload-url")) || "/files/upload";
  }

  function getFilesURL() {
    var el = document.getElementById("file-browser");
    return (el && el.getAttribute("data-files-url")) || "/files";
  }

  function getMkdirURL() {
    var el = document.getElementById("mkdir-btn");
    return (el && el.getAttribute("data-mkdir-url")) || "/files/mkdir";
  }

  // --- Upload Progress ---

  function showProgress() {
    var bar = document.getElementById("upload-progress");
    var fill = document.getElementById("upload-progress-fill");
    if (bar) bar.hidden = false;
    if (fill) fill.style.width = "0%";
  }

  function updateProgress(percent) {
    var fill = document.getElementById("upload-progress-fill");
    if (fill) fill.style.width = percent + "%";
  }

  function hideProgress() {
    setTimeout(function () {
      var bar = document.getElementById("upload-progress");
      var fill = document.getElementById("upload-progress-fill");
      if (bar) bar.hidden = true;
      if (fill) fill.style.width = "0%";
    }, PROGRESS_HIDE_DELAY_MS);
  }

  // --- File Upload (XHR for progress support) ---

  function uploadFormData(formData, url) {
    showProgress();

    var xhr = new XMLHttpRequest();
    xhr.open("POST", url, true);

    xhr.upload.addEventListener("progress", function (e) {
      if (e.lengthComputable) {
        var percent = Math.round((e.loaded / e.total) * 100);
        updateProgress(percent);
      }
    });

    xhr.addEventListener("load", function () {
      updateProgress(100);
      hideProgress();

      var data;
      try {
        data = JSON.parse(xhr.responseText);
      } catch (e) {
        showToast(MSG_UPLOAD_INVALID_RESPONSE, "error");
        return;
      }

      if (xhr.status !== 200) {
        showToast(data.message || MSG_UPLOAD_FAILED, "error");
        return;
      }

      if (data.failed > 0) {
        showToast(data.failed + MSG_UPLOAD_FILES_FAILED, "error");
      }
      if (data.uploaded > 0) {
        showToast(data.uploaded + MSG_UPLOAD_FILES_SUCCESS, "success");
      }
      refreshFileList();
    });

    xhr.addEventListener("error", function () {
      hideProgress();
      showToast(MSG_UPLOAD_NETWORK_ERROR, "error");
    });

    xhr.addEventListener("abort", function () {
      hideProgress();
      showToast(MSG_UPLOAD_CANCELLED, "info");
    });

    xhr.send(formData);
  }

  function uploadFiles(files, isClipboard) {
    if (!files || files.length === 0) return;

    var formData = new FormData();
    for (var i = 0; i < files.length; i++) {
      formData.append("file", files[i]);
    }

    var url =
      getUploadURL() + "?path=" + encodeURIComponent(getCurrentPath());
    if (isClipboard) url += "&clipboard=true";

    uploadFormData(formData, url);
  }

  // --- Directory Upload (webkitGetAsEntry API) ---

  // Recursively traverse a FileSystemEntry and collect {file, relpath} pairs.
  function traverseEntry(entry, basePath) {
    return new Promise(function (resolve) {
      if (entry.isFile) {
        entry.file(function (file) {
          resolve([{ file: file, relpath: basePath }]);
        }, function () {
          // Failed to read file — skip.
          resolve([]);
        });
      } else if (entry.isDirectory) {
        var reader = entry.createReader();
        var allEntries = [];

        // readEntries may not return all entries at once — read until empty.
        function readBatch() {
          reader.readEntries(function (entries) {
            if (entries.length === 0) {
              // All entries read — recurse into each.
              var promises = allEntries.map(function (child) {
                var childPath = basePath ? basePath + "/" + child.name : child.name;
                return traverseEntry(child, childPath);
              });
              Promise.all(promises).then(function (results) {
                var flat = [];
                results.forEach(function (arr) {
                  flat = flat.concat(arr);
                });
                resolve(flat);
              });
            } else {
              allEntries = allEntries.concat(Array.prototype.slice.call(entries));
              readBatch();
            }
          }, function () {
            resolve([]);
          });
        }
        readBatch();
      } else {
        resolve([]);
      }
    });
  }

  // Upload files with their relative paths (directory structure preserved).
  function uploadFilesWithPaths(filePathPairs) {
    if (!filePathPairs || filePathPairs.length === 0) return;

    var formData = new FormData();
    filePathPairs.forEach(function (pair) {
      formData.append("file", pair.file);
      // Extract the directory portion of the relpath (remove filename).
      var parts = pair.relpath.split("/");
      var dirPath = parts.length > 1 ? parts.slice(0, -1).join("/") : "";
      formData.append("relpath", dirPath);
    });

    var url =
      getUploadURL() + "?path=" + encodeURIComponent(getCurrentPath());

    uploadFormData(formData, url);
  }

  // Handle drop with directory support.
  // Collects webkitGetAsEntry results once to avoid repeated calls
  // (some browsers invalidate entries on subsequent calls).
  function handleDrop(dataTransfer) {
    if (!dataTransfer) return;

    var items = dataTransfer.items;
    var hasEntryAPI = items && items.length > 0 && items[0].webkitGetAsEntry;

    if (hasEntryAPI) {
      // Collect all entries in a single pass.
      var entries = [];
      var hasDir = false;
      for (var i = 0; i < items.length; i++) {
        var entry = items[i].webkitGetAsEntry();
        if (entry) {
          entries.push(entry);
          if (entry.isDirectory) hasDir = true;
        }
      }

      if (hasDir) {
        // Directory upload: traverse entries recursively.
        var promises = entries.map(function (entry) {
          return traverseEntry(entry, entry.name);
        });
        Promise.all(promises).then(function (results) {
          var flat = [];
          results.forEach(function (arr) {
            flat = flat.concat(arr);
          });
          if (flat.length > 0) {
            uploadFilesWithPaths(flat);
          }
        });
        return;
      }
    }

    // Flat file upload (no directory structure or no entry API).
    if (dataTransfer.files.length > 0) {
      uploadFiles(dataTransfer.files, false);
    }
  }

  function refreshFileList() {
    if (window.htmx) {
      var path = getCurrentPath();
      htmx.ajax("GET", getFilesURL() + "?path=" + encodeURIComponent(path), {
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
      handleDrop(e.dataTransfer);
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
      showToast(MSG_BOOKMARK_PREFIX + (path === "." ? MSG_BOOKMARK_HOME : path), "success");
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
        getMkdirURL() +
        "?path=" +
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
            showToast(MSG_MKDIR_PREFIX + result.data.name, "success");
            refreshFileList();
          } else {
            showToast(result.data.message || MSG_MKDIR_FAILED, "error");
          }
        })
        .catch(function (err) {
          showToast(MSG_MKDIR_ERROR_PREFIX + err.message, "error");
        });
    });
  }

  // --- Last-Dir Auto-Navigation ---

  function maybeNavigateToLastDir() {
    // Only act on bare "/" with no explicit path param.
    var params = new URLSearchParams(window.location.search);
    if (params.has("path")) return;

    var lastDir = getLastDir();
    if (!lastDir || lastDir === ".") return;

    // Use HTMX to navigate, update URL bar.
    if (window.htmx) {
      htmx.ajax("GET", getFilesURL() + "?path=" + encodeURIComponent(lastDir), {
        target: "#file-browser",
      });
      history.replaceState(null, "", "/?path=" + encodeURIComponent(lastDir));
    }
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
    // Navigate to last dir first, then save the resolved path.
    maybeNavigateToLastDir();
    saveLastDir(getCurrentPath());
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
