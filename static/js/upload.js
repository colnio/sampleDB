(function () {
    'use strict';

    // Initialise immediately if DOM is ready, otherwise wait.
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    function init() {
        document.querySelectorAll('[data-upload-zone]').forEach(initZone);
        observeZones();
        document.addEventListener('paste', onPaste);
    }

    // Attach drag-and-drop handlers to a single zone element.
    function initZone(zone) {
        if (zone.dataset.uploadInit) return;
        zone.dataset.uploadInit = '1';

        zone.addEventListener('dragover', function (e) {
            e.preventDefault();
            zone.classList.add('upload-drop-zone--over');
        });

        zone.addEventListener('dragleave', function (e) {
            // Only remove class when leaving the zone itself (not a child element).
            if (!zone.contains(e.relatedTarget)) {
                zone.classList.remove('upload-drop-zone--over');
            }
        });

        zone.addEventListener('drop', function (e) {
            e.preventDefault();
            zone.classList.remove('upload-drop-zone--over');
            var files = e.dataTransfer && e.dataTransfer.files;
            if (files && files.length) {
                uploadFiles(zone, files);
            }
        });
    }

    // Watch for new zones added to the DOM after HTMX swaps.
    function observeZones() {
        if (!window.MutationObserver) return;
        new MutationObserver(function (mutations) {
            mutations.forEach(function (m) {
                m.addedNodes.forEach(function (n) {
                    if (n.nodeType !== 1) return;
                    if (n.matches && n.matches('[data-upload-zone]')) {
                        initZone(n);
                    }
                    if (n.querySelectorAll) {
                        n.querySelectorAll('[data-upload-zone]').forEach(initZone);
                    }
                });
            });
        }).observe(document.body, { childList: true, subtree: true });
    }

    // Handle clipboard paste — upload any image items found.
    function onPaste(e) {
        var items = e.clipboardData && e.clipboardData.items;
        if (!items) return;

        var images = [];
        for (var i = 0; i < items.length; i++) {
            var item = items[i];
            if (item.type && item.type.indexOf('image/') === 0) {
                var file = item.getAsFile();
                if (file) images.push(file);
            }
        }

        if (!images.length) return;

        // Use the first zone found on the page.
        var zone = document.querySelector('[data-upload-zone]');
        if (zone) uploadFiles(zone, images);
    }

    // Upload files to the endpoint specified on the zone element.
    // On success, either swap the target element in the DOM or reload the page.
    function uploadFiles(zone, files) {
        var endpoint = zone.getAttribute('data-upload-endpoint');
        if (!endpoint) return;

        var targetSel = zone.getAttribute('data-upload-target') || '';
        var progress = zone.querySelector('[data-upload-progress]');
        var bar = zone.querySelector('[data-upload-progress-bar]');
        var label = zone.querySelector('[data-upload-progress-label]');

        var fd = new FormData();
        for (var i = 0; i < files.length; i++) {
            fd.append('files', files[i]);
        }

        if (progress) progress.style.display = '';

        var xhr = new XMLHttpRequest();

        xhr.upload.addEventListener('progress', function (e) {
            if (!e.lengthComputable) return;
            var pct = Math.round((e.loaded / e.total) * 100);
            if (bar) bar.style.width = pct + '%';
            if (label) label.textContent = pct + '%';
        });

        xhr.addEventListener('loadend', function () {
            if (progress) {
                progress.style.display = 'none';
                if (bar) bar.style.width = '0';
                if (label) label.textContent = '';
            }

            if (xhr.status < 200 || xhr.status >= 300) return;

            if (targetSel) {
                // Partial DOM swap: find the target and replace with server's version.
                var target = document.querySelector(targetSel);
                if (target) {
                    var tmp = document.createElement('div');
                    tmp.innerHTML = xhr.responseText;
                    var newEl = tmp.querySelector(targetSel);
                    if (newEl) {
                        target.outerHTML = newEl.outerHTML;
                        return;
                    }
                }
            }

            // Fallback: full page reload (used for wiki where no target selector is set).
            location.reload();
        });

        xhr.open('POST', endpoint);
        // Tell the server this is an HTMX-style request so it returns the partial.
        xhr.setRequestHeader('HX-Request', 'true');
        var csrfMeta = document.querySelector('meta[name="csrf-token"]');
        if (csrfMeta && csrfMeta.content) {
            xhr.setRequestHeader('X-CSRF-Token', csrfMeta.content);
        }
        xhr.send(fd);
    }
}());
