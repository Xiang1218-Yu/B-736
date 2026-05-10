// Main JavaScript

function bodyMessage(attributeName, fallback) {
    const body = document.body;
    if (!body || !body.dataset) {
        return fallback;
    }
    const value = body.dataset[attributeName];
    if (!value) {
        return fallback;
    }
    return value;
}

function ensureToastStack() {
    let stack = document.getElementById('toastStack');
    if (!stack) {
        stack = document.createElement('div');
        stack.id = 'toastStack';
        stack.className = 'toast-stack';
        document.body.appendChild(stack);
    }
    return stack;
}

function dismissToast(toast) {
    if (!toast || toast.dataset.dismissed === '1') {
        return;
    }
    toast.dataset.dismissed = '1';
    toast.style.opacity = '0';
    toast.style.transform = 'translateY(-8px)';
    setTimeout(function() {
        toast.remove();
    }, 250);
}

function showToast(message, type) {
    if (!message) {
        return;
    }

    const stack = ensureToastStack();
    const toast = document.createElement('div');
    toast.className = 'alert toast ' + (type === 'success' ? 'alert-success' : 'alert-error');
    toast.textContent = message;

    stack.appendChild(toast);

    setTimeout(function() {
        dismissToast(toast);
    }, 3200);
}

function ensureAppDialog() {
    let dialog = document.getElementById('appDialog');
    if (dialog) {
        return dialog;
    }

    dialog = document.createElement('div');
    dialog.id = 'appDialog';
    dialog.className = 'app-dialog-backdrop';
    dialog.setAttribute('hidden', 'hidden');
    dialog.innerHTML = [
        '<div class="app-dialog" role="dialog" aria-modal="true" aria-labelledby="appDialogTitle">',
        '  <div class="app-dialog-header">',
        '    <h3 id="appDialogTitle" class="app-dialog-title"></h3>',
        '    <button type="button" class="app-dialog-close" data-dialog-close aria-label="close">x</button>',
        '  </div>',
        '  <div id="appDialogMessage" class="app-dialog-message"></div>',
        '  <div class="app-dialog-actions">',
        '    <button type="button" class="btn btn-ghost btn-sm" data-dialog-cancel></button>',
        '    <button type="button" class="btn btn-primary btn-sm" data-dialog-confirm></button>',
        '  </div>',
        '</div>'
    ].join('');
    document.body.appendChild(dialog);
    return dialog;
}

function showAppDialog(options) {
    const opts = options || {};
    const mode = opts.mode || 'confirm';
    const title = opts.title || bodyMessage('msgNotice', 'Notice');
    const message = opts.message || '';
    const confirmText = opts.confirmText || (mode === 'confirm' ? bodyMessage('msgConfirm', 'Confirm') : bodyMessage('msgGotIt', 'Got it'));
    const cancelText = opts.cancelText || bodyMessage('msgCancel', 'Cancel');

    const dialog = ensureAppDialog();
    const titleNode = dialog.querySelector('#appDialogTitle');
    const messageNode = dialog.querySelector('#appDialogMessage');
    const confirmBtn = dialog.querySelector('[data-dialog-confirm]');
    const cancelBtn = dialog.querySelector('[data-dialog-cancel]');
    const closeBtn = dialog.querySelector('[data-dialog-close]');

    titleNode.textContent = title;
    messageNode.textContent = message;
    confirmBtn.textContent = confirmText;
    cancelBtn.textContent = cancelText;
    cancelBtn.style.display = mode === 'confirm' ? 'inline-flex' : 'none';

    dialog.removeAttribute('hidden');
    requestAnimationFrame(function() {
        dialog.classList.add('active');
    });

    return new Promise(function(resolve) {
        let settled = false;

        const cleanup = function(result) {
            if (settled) {
                return;
            }
            settled = true;
            dialog.classList.remove('active');
            setTimeout(function() {
                dialog.setAttribute('hidden', 'hidden');
            }, 160);
            dialog.removeEventListener('click', onBackdropClick);
            confirmBtn.removeEventListener('click', onConfirm);
            cancelBtn.removeEventListener('click', onCancel);
            closeBtn.removeEventListener('click', onCancel);
            document.removeEventListener('keydown', onKeydown);
            resolve(result);
        };

        const onConfirm = function() {
            cleanup(true);
        };

        const onCancel = function() {
            cleanup(false);
        };

        const onBackdropClick = function(e) {
            if (e.target === dialog) {
                cleanup(false);
            }
        };

        const onKeydown = function(e) {
            if (e.key === 'Escape') {
                cleanup(false);
            }
        };

        dialog.addEventListener('click', onBackdropClick);
        confirmBtn.addEventListener('click', onConfirm);
        cancelBtn.addEventListener('click', onCancel);
        closeBtn.addEventListener('click', onCancel);
        document.addEventListener('keydown', onKeydown);

        confirmBtn.focus();
    });
}

window.showAppConfirm = function(options) {
    return showAppDialog(Object.assign({ mode: 'confirm' }, options || {}));
};

window.showAppAlert = function(options) {
    return showAppDialog(Object.assign({ mode: 'alert' }, options || {}));
};

document.addEventListener('DOMContentLoaded', function() {
    const i18n = {
        passwordMismatch: bodyMessage('msgPasswordMismatch', 'Passwords do not match'),
        processing: bodyMessage('msgProcessing', 'Processing...'),
        chooseFile: bodyMessage('msgChooseFile', 'Choose File'),
        noFileSelected: bodyMessage('msgNoFileSelected', 'No file selected'),
        selectTags: bodyMessage('msgSelectTags', 'Select tags'),
        tagsSelected: bodyMessage('msgTagsSelected', '%d selected'),
        contactDetails: bodyMessage('msgContactDetails', 'Contact Details'),
        contactDefault: bodyMessage('msgContactDefault', 'Business Contact: WeChat GalaxyHub_BD / Email business@galaxyhub.example')
    };

    // Auto-hide server-rendered toasts
    document.querySelectorAll('.toast, .alert[data-toast]').forEach(function(toast, idx) {
        setTimeout(function() {
            dismissToast(toast);
        }, 3200 + idx * 160);
    });

    // Mobile menu toggle
    const mobileMenuBtn = document.getElementById('mobileMenuBtn');
    const nav = document.querySelector('.nav');

    if (mobileMenuBtn && nav) {
        const closeMobileNav = function() {
            nav.classList.remove('active');
            mobileMenuBtn.classList.remove('active');
        };

        mobileMenuBtn.addEventListener('click', function(e) {
            e.stopPropagation();
            nav.classList.toggle('active');
            mobileMenuBtn.classList.toggle('active');
        });

        nav.querySelectorAll('a').forEach(function(link) {
            link.addEventListener('click', closeMobileNav);
        });

        document.addEventListener('click', function(e) {
            if (!nav.contains(e.target) && !mobileMenuBtn.contains(e.target)) {
                closeMobileNav();
            }
        });

        window.addEventListener('resize', function() {
            if (window.innerWidth > 768) {
                closeMobileNav();
            }
        });
    }

    // Form validation
    const forms = document.querySelectorAll('form');
    forms.forEach(function(form) {
        form.addEventListener('submit', function(e) {
            const requiredFields = form.querySelectorAll('[required]');
            let isValid = true;

            requiredFields.forEach(function(field) {
                const fieldValue = typeof field.value === 'string' ? field.value.trim() : '';
                if (!fieldValue) {
                    isValid = false;
                    field.classList.add('error');
                    const clearEvent = field.tagName === 'SELECT' || field.type === 'file' ? 'change' : 'input';
                    field.addEventListener(clearEvent, function() {
                        field.classList.remove('error');
                    }, { once: true });
                }
            });

            const password = form.querySelector('#password');
            const confirmPassword = form.querySelector('#confirm_password');
            if (password && confirmPassword && password.value !== confirmPassword.value) {
                isValid = false;
                confirmPassword.classList.add('error');
                showToast(i18n.passwordMismatch, 'error');
            }

            if (!isValid) {
                e.preventDefault();
                return;
            }

            const submitBtn = form.querySelector('button[type="submit"]');
            if (submitBtn && !submitBtn.disabled) {
                submitBtn.disabled = true;
                const originalText = submitBtn.textContent;
                submitBtn.textContent = i18n.processing;

                // Re-enable if response is slow or blocked
                setTimeout(function() {
                    submitBtn.disabled = false;
                    submitBtn.textContent = originalText;
                }, 10000);
            }
        });
    });

    // Ad contact popups
    document.querySelectorAll('[data-ad-contact]').forEach(function(trigger) {
        trigger.addEventListener('click', function(e) {
            e.preventDefault();
            const title = this.dataset.adContactTitle || i18n.contactDetails;
            const message = this.dataset.adContact || i18n.contactDefault;

            if (typeof window.showAppAlert === 'function') {
                window.showAppAlert({ title: title, message: message });
                return;
            }
            showToast(message, 'success');
        });
    });

    // Resource filter custom selects
    const customSelects = Array.from(document.querySelectorAll('[data-custom-select]'));
    if (customSelects.length > 0) {
        const closeSelect = function(select) {
            select.classList.remove('open');
        };

        const closeAllSelects = function(except) {
            customSelects.forEach(function(select) {
                if (select !== except) {
                    closeSelect(select);
                }
            });
        };

        customSelects.forEach(function(select) {
            const trigger = select.querySelector('[data-select-trigger]');
            const label = select.querySelector('.custom-select-label');
            const hiddenInput = select.querySelector('input[type="hidden"]');
            const options = Array.from(select.querySelectorAll('.custom-select-option'));
            const form = select.closest('form');

            if (!trigger || !label || !hiddenInput || options.length === 0) {
                return;
            }

            const applyActiveOption = function(activeOption) {
                options.forEach(function(option) {
                    option.classList.toggle('active', option === activeOption);
                });

                if (activeOption) {
                    hiddenInput.value = activeOption.dataset.value || '';
                    label.textContent = activeOption.dataset.label || activeOption.textContent.trim();
                    return;
                }

                const fallback = options[0];
                hiddenInput.value = fallback.dataset.value || '';
                label.textContent = fallback.dataset.label || fallback.textContent.trim();
                fallback.classList.add('active');
            };

            const initialActive = options.find(function(option) {
                return option.classList.contains('active');
            });
            applyActiveOption(initialActive || null);

            trigger.addEventListener('click', function(e) {
                e.preventDefault();
                const willOpen = !select.classList.contains('open');
                closeAllSelects(select);
                if (willOpen) {
                    select.classList.add('open');
                } else {
                    closeSelect(select);
                }
            });

            options.forEach(function(option) {
                option.addEventListener('click', function(e) {
                    e.preventDefault();
                    applyActiveOption(option);
                    closeSelect(select);
                    if (form) {
                        form.submit();
                    }
                });
            });
        });

        document.addEventListener('click', function(e) {
            customSelects.forEach(function(select) {
                if (!select.contains(e.target)) {
                    closeSelect(select);
                }
            });
        });

        document.addEventListener('keydown', function(e) {
            if (e.key === 'Escape') {
                closeAllSelects(null);
            }
        });
    }

    // Publish form custom file inputs
    document.querySelectorAll('input[type="file"].form-input').forEach(function(input) {
        if (input.dataset.customized === '1') {
            return;
        }
        input.dataset.customized = '1';
        input.classList.add('custom-file-native');

        const shell = document.createElement('div');
        shell.className = 'custom-file-shell';
        shell.innerHTML = [
            '<button type="button" class="custom-file-trigger"></button>',
            '<span class="custom-file-label"></span>'
        ].join('');
        input.insertAdjacentElement('afterend', shell);

        const trigger = shell.querySelector('.custom-file-trigger');
        const label = shell.querySelector('.custom-file-label');
        if (!trigger || !label) {
            return;
        }

        trigger.textContent = i18n.chooseFile;

        const syncFileLabel = function() {
            if (input.files && input.files.length > 0) {
                label.textContent = input.files[0].name;
                shell.classList.add('has-file');
                return;
            }
            label.textContent = i18n.noFileSelected;
            shell.classList.remove('has-file');
        };

        syncFileLabel();

        trigger.addEventListener('click', function(e) {
            e.preventDefault();
            input.click();
        });

        shell.addEventListener('click', function(e) {
            if (e.target === shell || e.target === label) {
                input.click();
            }
        });

        input.addEventListener('change', function() {
            input.classList.remove('error');
            syncFileLabel();
        });
    });

    // Publish form custom single selects
    const singleSelectNatives = Array.from(document.querySelectorAll('form.publish-form select.form-input:not([multiple])'));
    if (singleSelectNatives.length > 0) {
        const singleSelectInstances = [];

        const closeSingleSelect = function(instance) {
            instance.root.classList.remove('open');
            instance.trigger.setAttribute('aria-expanded', 'false');
        };

        const closeAllSingleSelects = function(except) {
            singleSelectInstances.forEach(function(instance) {
                if (instance !== except) {
                    closeSingleSelect(instance);
                }
            });
        };

        singleSelectNatives.forEach(function(select) {
            if (select.dataset.customized === '1') {
                return;
            }
            select.dataset.customized = '1';
            select.classList.add('is-customized-single');

            const root = document.createElement('div');
            root.className = 'publish-single-select-custom';
            root.innerHTML = [
                '<button type="button" class="publish-single-select-trigger" data-single-trigger aria-haspopup="listbox" aria-expanded="false">',
                '  <span class="publish-single-select-text" data-single-label></span>',
                '  <span class="publish-single-select-arrow" aria-hidden="true"></span>',
                '</button>',
                '<div class="publish-single-select-menu" data-single-menu role="listbox"></div>'
            ].join('');
            select.insertAdjacentElement('afterend', root);

            const trigger = root.querySelector('[data-single-trigger]');
            const label = root.querySelector('[data-single-label]');
            const menu = root.querySelector('[data-single-menu]');
            if (!trigger || !label || !menu) {
                return;
            }

            const optionPairs = [];
            Array.from(select.options).forEach(function(option) {
                const item = document.createElement('button');
                item.type = 'button';
                item.className = 'publish-single-select-option';
                item.setAttribute('role', 'option');
                item.textContent = option.textContent.trim();
                if (option.disabled) {
                    item.disabled = true;
                }
                menu.appendChild(item);
                optionPairs.push({ option: option, item: item, text: item.textContent });
            });

            const syncSelectedState = function() {
                let selectedPair = optionPairs.find(function(pair) {
                    return pair.option.selected;
                });

                if (!selectedPair && optionPairs.length > 0) {
                    selectedPair = optionPairs[0];
                    selectedPair.option.selected = true;
                }

                optionPairs.forEach(function(pair) {
                    const isActive = selectedPair && pair.option === selectedPair.option;
                    pair.item.classList.toggle('active', isActive);
                    pair.item.setAttribute('aria-selected', isActive ? 'true' : 'false');
                });

                if (selectedPair) {
                    label.textContent = selectedPair.text;
                }
            };

            optionPairs.forEach(function(pair) {
                pair.item.addEventListener('click', function(e) {
                    e.preventDefault();
                    if (pair.item.disabled) {
                        return;
                    }
                    if (select.value !== pair.option.value) {
                        select.value = pair.option.value;
                        select.dispatchEvent(new Event('change', { bubbles: true }));
                    } else {
                        select.dispatchEvent(new Event('change', { bubbles: true }));
                    }
                    syncSelectedState();
                    closeSingleSelect({ root: root, trigger: trigger });
                });
            });

            trigger.addEventListener('click', function(e) {
                e.preventDefault();
                if (trigger.disabled) {
                    return;
                }
                const willOpen = !root.classList.contains('open');
                closeAllSingleSelects(null);
                if (willOpen) {
                    root.classList.add('open');
                    trigger.setAttribute('aria-expanded', 'true');
                } else {
                    closeSingleSelect({ root: root, trigger: trigger });
                }
            });

            select.addEventListener('change', function() {
                select.classList.remove('error');
                syncSelectedState();
            });

            if (select.disabled) {
                trigger.disabled = true;
            }

            syncSelectedState();
            singleSelectInstances.push({ root: root, trigger: trigger });
        });

        document.addEventListener('click', function(e) {
            singleSelectInstances.forEach(function(instance) {
                if (!instance.root.contains(e.target)) {
                    closeSingleSelect(instance);
                }
            });
        });

        document.addEventListener('keydown', function(e) {
            if (e.key === 'Escape') {
                closeAllSingleSelects(null);
            }
        });
    }

    // Publish form custom tag multi-select
    const multiSelectNatives = Array.from(document.querySelectorAll('select.publish-multi-select'));
    if (multiSelectNatives.length > 0) {
        const multiSelectInstances = [];

        const closeMultiSelect = function(instance) {
            instance.root.classList.remove('open');
            instance.trigger.setAttribute('aria-expanded', 'false');
        };

        const closeAllMultiSelects = function(except) {
            multiSelectInstances.forEach(function(instance) {
                if (instance !== except) {
                    closeMultiSelect(instance);
                }
            });
        };

        const formatSelectedCount = function(count) {
            const template = i18n.tagsSelected || '%d selected';
            if (template.indexOf('%d') >= 0) {
                return template.replace('%d', String(count));
            }
            return String(count) + ' ' + template;
        };

        multiSelectNatives.forEach(function(select) {
            if (select.dataset.customized === '1') {
                return;
            }
            select.dataset.customized = '1';
            select.classList.add('is-customized');

            const root = document.createElement('div');
            root.className = 'publish-multi-select-custom';
            root.innerHTML = [
                '<button type="button" class="publish-multi-select-trigger" data-multi-trigger aria-haspopup="listbox" aria-expanded="false">',
                '  <span class="publish-multi-select-text" data-multi-label></span>',
                '  <span class="publish-multi-select-arrow" aria-hidden="true"></span>',
                '</button>',
                '<div class="publish-multi-select-menu" data-multi-menu role="listbox" aria-multiselectable="true"></div>'
            ].join('');
            select.insertAdjacentElement('afterend', root);

            const trigger = root.querySelector('[data-multi-trigger]');
            const label = root.querySelector('[data-multi-label]');
            const menu = root.querySelector('[data-multi-menu]');
            if (!trigger || !label || !menu) {
                return;
            }

            const optionPairs = [];
            Array.from(select.options).forEach(function(option) {
                const item = document.createElement('button');
                item.type = 'button';
                item.className = 'publish-multi-select-option';
                item.setAttribute('role', 'option');
                item.innerHTML = [
                    '<span class="publish-multi-select-check" aria-hidden="true"></span>',
                    '<span class="publish-multi-select-option-text"></span>'
                ].join('');
                const optionText = option.textContent.trim();
                item.querySelector('.publish-multi-select-option-text').textContent = optionText;
                menu.appendChild(item);
                optionPairs.push({ option: option, item: item, text: optionText });
            });

            const syncSelectedState = function() {
                const selectedTexts = [];
                optionPairs.forEach(function(pair) {
                    const isSelected = !!pair.option.selected;
                    pair.item.classList.toggle('selected', isSelected);
                    pair.item.setAttribute('aria-selected', isSelected ? 'true' : 'false');
                    if (isSelected) {
                        selectedTexts.push(pair.text);
                    }
                });

                if (selectedTexts.length === 0) {
                    label.textContent = i18n.selectTags;
                    return;
                }

                if (selectedTexts.length <= 2) {
                    label.textContent = selectedTexts.join(', ');
                    return;
                }

                label.textContent = formatSelectedCount(selectedTexts.length);
            };

            if (optionPairs.length === 0) {
                trigger.disabled = true;
                label.textContent = i18n.selectTags;
                const empty = document.createElement('div');
                empty.className = 'publish-multi-select-empty';
                empty.textContent = i18n.selectTags;
                menu.appendChild(empty);
            }

            optionPairs.forEach(function(pair) {
                pair.item.addEventListener('click', function(e) {
                    e.preventDefault();
                    pair.option.selected = !pair.option.selected;
                    syncSelectedState();
                    select.dispatchEvent(new Event('change', { bubbles: true }));
                });
            });

            trigger.addEventListener('click', function(e) {
                e.preventDefault();
                const willOpen = !root.classList.contains('open');
                closeAllMultiSelects(null);
                if (willOpen) {
                    root.classList.add('open');
                    trigger.setAttribute('aria-expanded', 'true');
                } else {
                    closeMultiSelect({ root: root, trigger: trigger });
                }
            });

            select.addEventListener('change', syncSelectedState);
            syncSelectedState();

            multiSelectInstances.push({ root: root, trigger: trigger });
        });

        document.addEventListener('click', function(e) {
            multiSelectInstances.forEach(function(instance) {
                if (!instance.root.contains(e.target)) {
                    closeMultiSelect(instance);
                }
            });
        });

        document.addEventListener('keydown', function(e) {
            if (e.key === 'Escape') {
                closeAllMultiSelects(null);
            }
        });
    }

    // Smooth scroll for anchor links
    document.querySelectorAll('a[href^="#"]').forEach(function(anchor) {
        anchor.addEventListener('click', function(e) {
            e.preventDefault();
            const target = document.querySelector(this.getAttribute('href'));
            if (target) {
                target.scrollIntoView({ behavior: 'smooth', block: 'start' });
            }
        });
    });

    // Tooltips
    document.querySelectorAll('[data-tooltip]').forEach(function(trigger) {
        trigger.addEventListener('mouseenter', function() {
            const tooltip = document.createElement('div');
            tooltip.className = 'tooltip';
            tooltip.textContent = this.dataset.tooltip;
            document.body.appendChild(tooltip);

            const rect = this.getBoundingClientRect();
            tooltip.style.top = (rect.top - tooltip.offsetHeight - 8) + 'px';
            tooltip.style.left = (rect.left + rect.width / 2 - tooltip.offsetWidth / 2) + 'px';
        });

        trigger.addEventListener('mouseleave', function() {
            const tooltip = document.querySelector('.tooltip');
            if (tooltip) {
                tooltip.remove();
            }
        });
    });
});

// Utility: Format date
function formatDate(dateString) {
    const date = new Date(dateString);
    const locale = document.documentElement.lang || 'zh-CN';
    return date.toLocaleDateString(locale, {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit'
    });
}

// Utility: Truncate text
function truncate(text, length) {
    if (text.length <= length) return text;
    return text.substring(0, length) + '...';
}
