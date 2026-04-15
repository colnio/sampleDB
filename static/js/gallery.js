(function () {
    'use strict';

    var state = {
        sets: [],
        current: { setIndex: -1, imgIndex: -1 },
        modal: null,
        img: null,
        caption: null,
        counter: null,
        prevBtn: null,
        nextBtn: null
    };

    function init() {
        buildSets();
        createModal();
        attachHandlers();

        // Re-scan after HTMX content swaps (e.g. attachment upload/delete)
        document.body.addEventListener('htmx:afterSwap', function () {
            buildSets();
            attachHandlers();
        });
    }

    function buildSets() {
        state.sets = [];
        document.querySelectorAll('.gallery-set').forEach(function (setEl) {
            var thumbs = Array.prototype.slice.call(setEl.querySelectorAll('[data-gallery-src]'));
            if (thumbs.length === 0) return;
            var set = { thumbs: thumbs, srcs: [], names: [] };
            thumbs.forEach(function (t) {
                set.srcs.push(t.dataset.gallerySrc);
                set.names.push(t.dataset.galleryName || '');
            });
            state.sets.push(set);
        });
    }

    function attachHandlers() {
        state.sets.forEach(function (set, setIdx) {
            set.thumbs.forEach(function (thumb, imgIdx) {
                if (thumb.dataset.galleryInit) return;
                thumb.dataset.galleryInit = '1';
                thumb.addEventListener('click', function (e) {
                    e.preventDefault();
                    open(setIdx, imgIdx);
                });
            });
        });
    }

    function createModal() {
        if (document.getElementById('gallery-modal')) return;

        var modal = document.createElement('div');
        modal.id = 'gallery-modal';
        modal.className = 'gallery-modal';
        modal.setAttribute('aria-hidden', 'true');
        modal.setAttribute('role', 'dialog');
        modal.setAttribute('aria-modal', 'true');
        modal.setAttribute('aria-label', 'Image gallery');

        modal.innerHTML =
            '<div class="gallery-modal__overlay"></div>' +
            '<div class="gallery-modal__content">' +
            '  <button type="button" class="gallery-modal__close" aria-label="Close gallery">\u00d7</button>' +
            '  <button type="button" class="gallery-modal__prev" aria-label="Previous image">&#8249;</button>' +
            '  <button type="button" class="gallery-modal__next" aria-label="Next image">&#8250;</button>' +
            '  <div class="gallery-modal__img-wrap">' +
            '    <img class="gallery-modal__img" src="" alt="">' +
            '  </div>' +
            '  <div class="gallery-modal__footer">' +
            '    <p class="gallery-modal__caption"></p>' +
            '    <span class="gallery-modal__counter"></span>' +
            '  </div>' +
            '</div>';

        document.body.appendChild(modal);
        state.modal    = modal;
        state.img      = modal.querySelector('.gallery-modal__img');
        state.caption  = modal.querySelector('.gallery-modal__caption');
        state.counter  = modal.querySelector('.gallery-modal__counter');
        state.prevBtn  = modal.querySelector('.gallery-modal__prev');
        state.nextBtn  = modal.querySelector('.gallery-modal__next');

        modal.querySelector('.gallery-modal__overlay').addEventListener('click', close);
        modal.querySelector('.gallery-modal__close').addEventListener('click', close);
        state.prevBtn.addEventListener('click', function (e) { e.stopPropagation(); navigate(-1); });
        state.nextBtn.addEventListener('click', function (e) { e.stopPropagation(); navigate(1); });

        document.addEventListener('keyup', function (e) {
            if (!state.modal || state.modal.getAttribute('aria-hidden') === 'true') return;
            if (e.key === 'Escape')      close();
            else if (e.key === 'ArrowLeft')  navigate(-1);
            else if (e.key === 'ArrowRight') navigate(1);
        });
    }

    function open(setIndex, imgIndex) {
        if (!state.modal) return;
        state.current.setIndex = setIndex;
        state.current.imgIndex = imgIndex;
        showImage();
        state.modal.classList.add('gallery-modal--open');
        state.modal.setAttribute('aria-hidden', 'false');
        state.modal.querySelector('.gallery-modal__close').focus();
    }

    function close() {
        if (!state.modal) return;
        state.modal.classList.remove('gallery-modal--open');
        state.modal.setAttribute('aria-hidden', 'true');
        state.img.src = '';
        state.img.alt = '';
    }

    function navigate(direction) {
        var set = state.sets[state.current.setIndex];
        if (!set) return;
        var next = state.current.imgIndex + direction;
        if (next < 0) next = set.srcs.length - 1;
        if (next >= set.srcs.length) next = 0;
        state.current.imgIndex = next;
        showImage();
    }

    function showImage() {
        var set = state.sets[state.current.setIndex];
        if (!set) return;
        var i   = state.current.imgIndex;
        var src = set.srcs[i];
        var name = set.names[i];

        state.img.src         = src;
        state.img.alt         = name;
        state.caption.textContent = name;
        state.counter.textContent = (i + 1) + ' / ' + set.srcs.length;

        var single = set.srcs.length <= 1;
        state.prevBtn.style.display = single ? 'none' : '';
        state.nextBtn.style.display = single ? 'none' : '';
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
