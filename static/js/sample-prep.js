(function () {
    'use strict';

    let samplePrepEditor = null;
    let wikiContentEditor = null;

    function destroySamplePrepEditor() {
        if (samplePrepEditor) {
            samplePrepEditor.toTextArea();
            samplePrepEditor = null;
        }
    }

    function destroyWikiEditor() {
        if (wikiContentEditor) {
            wikiContentEditor.toTextArea();
            wikiContentEditor = null;
        }
    }

    function initSamplePrepEditor() {
        const textarea = document.getElementById('sample-prep-editor');

        if (!textarea || typeof EasyMDE === 'undefined') {
            destroySamplePrepEditor();
            return;
        }

        destroySamplePrepEditor();

        samplePrepEditor = new EasyMDE({
            element: textarea,
            spellChecker: false,
            toolbar: [
                'bold', 'italic', 'heading', '|',
                'unordered-list', 'ordered-list', '|',
                'link', 'code', 'quote', '|',
                {
                    name: 'guide',
                    action: 'https://www.markdownguide.org/basic-syntax/',
                    className: 'fa fa-question-circle',
                    title: 'Markdown Guide'
                }
            ],
            status: false,
            renderingConfig: {
                singleLineBreaks: false,
                codeSyntaxHighlighting: true
            }
        });
    }

    function initWikiContentEditor() {
        const textarea = document.getElementById('wiki-content-editor');

        if (!textarea || typeof EasyMDE === 'undefined') {
            destroyWikiEditor();
            return;
        }

        destroyWikiEditor();

        wikiContentEditor = new EasyMDE({
            element: textarea,
            spellChecker: false,
            toolbar: [
                'bold', 'italic', 'heading', '|',
                'unordered-list', 'ordered-list', '|',
                'link', 'code', 'quote', '|',
                {
                    name: 'guide',
                    action: 'https://www.markdownguide.org/basic-syntax/',
                    className: 'fa fa-question-circle',
                    title: 'Markdown Guide'
                }
            ],
            status: false,
            renderingConfig: {
                singleLineBreaks: false,
                codeSyntaxHighlighting: true
            }
        });
    }

    function handleSwapTarget(target) {
        if (!target) {
            return;
        }

        // Check if the swapped element contains the sample-prep-panel or is the panel itself
        const samplePrepPanel = target.id === 'sample-prep-panel' ? target : target.querySelector?.('#sample-prep-panel') || target.closest?.('#sample-prep-panel');
        
        // Check if the swapped element contains the article-content-panel or is the panel itself
        const articleContentPanel = target.id === 'article-content-panel' ? target : target.querySelector?.('#article-content-panel') || target.closest?.('#article-content-panel');
        
        if (samplePrepPanel) {
            // Use setTimeout to ensure DOM is fully updated after HTMX swap
            setTimeout(initSamplePrepEditor, 10);
        } else if (articleContentPanel) {
            setTimeout(initWikiContentEditor, 10);
        } else if (target.id === 'page-root') {
            setTimeout(function() {
                initSamplePrepEditor();
                initWikiContentEditor();
            }, 10);
        }
    }

    document.addEventListener('DOMContentLoaded', function() {
        initSamplePrepEditor();
        initWikiContentEditor();
    });
    document.addEventListener('htmx:afterSwap', function (event) {
        const target = event.detail?.target || event.target;
        handleSwapTarget(target);
    });
})();
