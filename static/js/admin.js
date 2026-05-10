// Admin JavaScript

document.addEventListener('DOMContentLoaded', function() {
    const body = document.body;
    const confirmFallback = body.dataset.msgConfirmAction || 'Are you sure?';
    const confirmTitle = body.dataset.msgConfirmTitle || 'Confirm Action';
    const confirmText = body.dataset.msgConfirm || 'Confirm';
    const cancelText = body.dataset.msgCancel || 'Cancel';

    const showConfirmDialog = function(message) {
        if (typeof window.showAppConfirm === 'function') {
            return window.showAppConfirm({
                title: confirmTitle,
                message: message,
                confirmText: confirmText,
                cancelText: cancelText
            });
        }
        return Promise.resolve(false);
    };

    // Confirm delete actions
    const confirmTriggers = document.querySelectorAll('[data-confirm]');
    confirmTriggers.forEach(function(el) {
        const message = el.dataset.confirm || confirmFallback;

        if (el.tagName === 'FORM') {
            el.addEventListener('submit', function(e) {
                e.preventDefault();
                showConfirmDialog(message).then(function(confirmed) {
                    if (confirmed) {
                        HTMLFormElement.prototype.submit.call(el);
                    }
                });
            });
            return;
        }

        el.addEventListener('click', function(e) {
            e.preventDefault();
            showConfirmDialog(message).then(function(confirmed) {
                if (!confirmed) {
                    return;
                }

                if ((el.tagName === 'BUTTON' || el.tagName === 'INPUT') && el.form) {
                    HTMLFormElement.prototype.submit.call(el.form);
                    return;
                }
                if (el.tagName === 'A' && el.href) {
                    window.location.href = el.href;
                    return;
                }
                const parentForm = el.closest('form');
                if (parentForm) {
                    HTMLFormElement.prototype.submit.call(parentForm);
                }
            });
        });
    });

    // Auto-submit on select change (for filters)
    const autoSubmitSelects = document.querySelectorAll('.auto-submit');
    autoSubmitSelects.forEach(function(select) {
        select.addEventListener('change', function() {
            this.closest('form').submit();
        });
    });

    // Inline edit forms
    const inlineEditForms = document.querySelectorAll('.inline-edit-form');
    inlineEditForms.forEach(function(form) {
        const input = form.querySelector('input[type="text"]');
        if (!input) {
            return;
        }
        const originalValue = input.value;

        input.addEventListener('blur', function() {
            if (this.value !== originalValue) {
                // Highlight changed
                this.style.borderColor = 'var(--warning)';
            }
        });
    });

    // Table row hover effect
    const tableRows = document.querySelectorAll('.admin-table tbody tr');
    tableRows.forEach(function(row) {
        row.addEventListener('mouseenter', function() {
            this.style.backgroundColor = 'var(--bg-tertiary)';
        });
        row.addEventListener('mouseleave', function() {
            this.style.backgroundColor = '';
        });
    });

    // Keyboard shortcuts
    document.addEventListener('keydown', function(e) {
        // Ctrl/Cmd + S to save current form
        if ((e.ctrlKey || e.metaKey) && e.key === 's') {
            const activeForm = document.querySelector('.admin-form');
            if (activeForm) {
                e.preventDefault();
                activeForm.submit();
            }
        }
    });

    // Mobile nav toggle
    const mobileToggle = document.getElementById('adminMobileToggle');
    const mobileNav = document.getElementById('adminMobileNav');

    if (mobileToggle && mobileNav) {
        const closeMobileNav = function() {
            mobileNav.classList.remove('active');
        };

        mobileToggle.addEventListener('click', function(e) {
            e.stopPropagation();
            mobileNav.classList.toggle('active');
        });

        mobileNav.querySelectorAll('a').forEach(function(link) {
            link.addEventListener('click', closeMobileNav);
        });

        document.addEventListener('click', function(e) {
            if (!mobileNav.contains(e.target) && !mobileToggle.contains(e.target)) {
                closeMobileNav();
            }
        });

        window.addEventListener('resize', function() {
            if (window.innerWidth > 768) {
                closeMobileNav();
            }
        });
    }
});
