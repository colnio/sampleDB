// Admin panel JavaScript with improved UX, accessibility, and error handling

(function() {
    'use strict';

    let currentEquipmentId = null;
    let updateAccessTimeout = null;
    const DEBOUNCE_DELAY = 500;

    // Utility: Show flash message
    function showFlashMessage(message, type = 'success', duration = 3000) {
        const flashRegion = document.querySelector('.flash-region');
        if (!flashRegion) return;

        flashRegion.querySelectorAll('.alert-temp').forEach(el => el.remove());

        const msg = document.createElement('div');
        msg.className = `alert alert-${type} alert-temp`;
        msg.setAttribute('role', 'alert');
        msg.setAttribute('aria-live', 'polite');
        msg.textContent = message;
        flashRegion.appendChild(msg);

        setTimeout(() => {
            msg.style.transition = 'opacity 0.3s ease';
            msg.style.opacity = '0';
            setTimeout(() => msg.remove(), 300);
        }, duration);
    }

    // Utility: Show error message
    function showError(message) {
        showFlashMessage(message, 'error', 5000);
    }

    // Utility: Set loading state on button
    function setButtonLoading(button, loading) {
        if (loading) {
            button.disabled = true;
            button.classList.add('button--loading');
            button.setAttribute('aria-busy', 'true');
        } else {
            button.disabled = false;
            button.classList.remove('button--loading');
            button.removeAttribute('aria-busy');
        }
    }

    // Utility: Debounce function
    function debounce(func, delay) {
        return function(...args) {
            clearTimeout(updateAccessTimeout);
            updateAccessTimeout = setTimeout(() => func.apply(this, args), delay);
        };
    }

    // Update user access with debouncing
    function updateAccessImmediate(userId) {
        const row = document.querySelector(`.user-card[data-user-id="${userId}"]`);
        if (!row) {
            return;
        }

        const approvedToggle = row.querySelector('.approved-toggle');
        const equipmentSelect = row.querySelector('.equipment-select');
        const groupSelect = row.querySelector('.group-select');

        const approved = approvedToggle?.checked ?? false;
        const selectedEquipment = Array.from(equipmentSelect?.selectedOptions ?? [])
            .map(option => parseInt(option.value, 10))
            .filter(Number.isInteger);

        const rawGroupValue = groupSelect?.value ?? '';
        const groupValue = rawGroupValue.trim() || null;
        const groupLabel = groupSelect?.selectedOptions?.[0]?.textContent?.trim() ?? '';

        // Update badge UI immediately for better UX
        const badge = row.querySelector('.badge');
        const userMeta = row.querySelector('.user-meta');
        if (groupValue && userMeta) {
            if (badge) {
                badge.textContent = groupLabel;
            } else {
                const newBadge = document.createElement('span');
                newBadge.className = 'badge';
                newBadge.textContent = groupLabel;
                userMeta.appendChild(newBadge);
            }
        } else if (badge) {
            badge.remove();
        }

        fetch('/admin/update-access', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                user_id: userId,
                approved: approved,
                group_name: groupValue,
                equipment: selectedEquipment
            })
        })
        .then(response => {
            if (!response.ok) {
                return response.text().then(text => {
                    throw new Error(text || 'Failed to update access');
                });
            }
            showFlashMessage('User settings updated', 'success', 2000);
        })
        .catch(error => {
            console.error('Error updating access:', error);
            showError(error.message || 'Failed to update access rights. Please refresh and try again.');
            // Revert UI changes on error
            location.reload();
        });
    }

    // Debounced version of updateAccess
    const updateAccess = debounce(updateAccessImmediate, DEBOUNCE_DELAY);

    // Modal management
    function showModal(modalId) {
        const modal = document.getElementById(modalId);
        if (!modal) return;

        modal.classList.add('show');
        modal.setAttribute('aria-hidden', 'false');

        // Focus management
        const firstInput = modal.querySelector('input, button, select, textarea');
        if (firstInput) {
            setTimeout(() => firstInput.focus(), 100);
        }

        // Prevent body scroll
        document.body.style.overflow = 'hidden';
    }

    function closeModal(modalId) {
        const modal = document.getElementById(modalId);
        if (!modal) return;

        modal.classList.remove('show');
        modal.setAttribute('aria-hidden', 'true');
        document.body.style.overflow = '';

        // Reset forms
        const form = modal.querySelector('form');
        if (form) {
            form.reset();
        }
    }

    // Equipment modal
    function showEquipmentModal() {
        showModal('equipment-modal');
    }

    function closeEquipmentModal() {
        closeModal('equipment-modal');
    }

    // Report modal
    function showReportModal(equipmentId, equipmentName) {
        currentEquipmentId = equipmentId;
        const modal = document.getElementById('report-modal');
        if (modal && equipmentName) {
            const title = modal.querySelector('.modal-header h3');
            if (title) {
                title.textContent = `Download Usage Report: ${equipmentName}`;
            }
        }
        showModal('report-modal');
    }

    function closeReportModal() {
        closeModal('report-modal');
        currentEquipmentId = null;
    }

    function downloadReport() {
        const startDateInput = document.getElementById('start_date');
        const endDateInput = document.getElementById('end_date');
        const startDate = startDateInput?.value;
        const endDate = endDateInput?.value;

        if (!startDate || !endDate) {
            showError('Please select both start and end dates');
            startDateInput?.focus();
            return;
        }

        if (new Date(endDate) < new Date(startDate)) {
            showError('End date must be after start date');
            endDateInput?.focus();
            return;
        }

        // Validate date range (max 1 year)
        const start = new Date(startDate);
        const end = new Date(endDate);
        const daysDiff = (end - start) / (1000 * 60 * 60 * 24);
        if (daysDiff > 365) {
            showError('Date range cannot exceed 365 days');
            return;
        }

        window.location.href = `/admin/equipment-report?id=${currentEquipmentId}&start=${startDate}&end=${endDate}`;
        closeReportModal();
    }

    // Delete equipment
    function confirmDeleteEquipment(equipmentId, equipmentName, buttonElement) {
        if (confirm(`Remove ${equipmentName}? This will also clear related bookings and permissions.`)) {
            const button = buttonElement || document.activeElement;
            if (button && button.tagName === 'BUTTON') {
                setButtonLoading(button, true);
            }

            fetch(`/admin/delete-equipment/${equipmentId}`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
            })
            .then(response => {
                if (!response.ok) {
                    return response.text().then(text => {
                        throw new Error(text || 'Failed to delete equipment');
                    });
                }
                showFlashMessage('Equipment removed successfully', 'success', 2000);
                setTimeout(() => location.reload(), 2000);
            })
            .catch(error => {
                console.error('Error:', error);
                showError(error.message || 'Failed to delete equipment');
                if (button && button.tagName === 'BUTTON') {
                    setButtonLoading(button, false);
                }
            });
        }
    }

    // Reset password
    function resetPassword(userId, buttonElement) {
        const row = document.querySelector(`.user-card[data-user-id="${userId}"]`);
        const username = row?.dataset.username ?? 'this user';

        if (!confirm(`Reset password for ${username}? They will need to change it after signing in.`)) {
            return;
        }

        const button = buttonElement || document.activeElement;
        if (button && button.tagName === 'BUTTON') {
            setButtonLoading(button, true);
        }

        fetch('/admin/reset-password', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({ user_id: userId }),
        })
        .then(response => {
            if (!response.ok) {
                return response.text().then(text => {
                    throw new Error(text || 'Failed to reset password');
                });
            }
            return response.json();
        })
        .then(data => {
            const password = data?.password;
            if (!password) {
                throw new Error('Server did not provide a new password');
            }

            const showPrompt = () => {
                window.prompt('Temporary password generated. Copy it now:', password);
            };

            if (navigator.clipboard && window.isSecureContext) {
                navigator.clipboard.writeText(password).then(() => {
                    alert(`Temporary password for ${username}: ${password}`);
                }).catch(() => {
                    showPrompt();
                });
            } else {
                showPrompt();
            }

            showFlashMessage(`Temporary password generated for ${username}.`, 'success', 6000);
        })
        .catch(error => {
            console.error('Error:', error);
            showError(error.message || 'Failed to reset password. Please try again.');
        })
        .finally(() => {
            if (button && button.tagName === 'BUTTON') {
                setButtonLoading(button, false);
            }
        });
    }

    // Remove user
    function confirmRemoveUser(userId, buttonElement) {
        const row = document.querySelector(`.user-card[data-user-id="${userId}"]`);
        const username = row?.dataset.username ?? 'this user';

        if (!confirm(`Remove ${username}? They will no longer be able to sign in.`)) {
            return;
        }

        const button = buttonElement || document.activeElement;
        if (button && button.tagName === 'BUTTON') {
            setButtonLoading(button, true);
        }

        fetch('/admin/delete-user', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({ user_id: userId }),
        })
        .then(response => {
            if (!response.ok) {
                if (response.status === 400) {
                    return response.text().then(text => {
                        throw new Error(text || 'Invalid request');
                    });
                }
                if (response.status === 404) {
                    throw new Error('User not found or already removed');
                }
                throw new Error('Failed to remove user');
            }
            return response.json().catch(() => ({}));
        })
        .then(() => {
            showFlashMessage('User removed', 'success', 2000);
            setTimeout(() => row?.remove(), 2000);
        })
        .catch(error => {
            console.error('Error:', error);
            showError(error.message || 'Failed to remove user. Please refresh and try again.');
            if (button && button.tagName === 'BUTTON') {
                setButtonLoading(button, false);
            }
        });
    }

    // Keyboard navigation for modals
    function handleModalKeyboard(event) {
        if (event.key === 'Escape') {
            const openModal = document.querySelector('.modal.show');
            if (openModal) {
                closeModal(openModal.id);
            }
        }
    }

    // Close modal on backdrop click
    function handleModalBackdropClick(event) {
        if (event.target.classList.contains('modal')) {
            closeModal(event.target.id);
        }
    }

    // Initialize on DOM ready
    function init() {
        // Set up modal event listeners
        document.addEventListener('click', (event) => {
            if (event.target.classList.contains('close') || event.target.closest('.close')) {
                const modal = event.target.closest('.modal') || event.target.closest('[data-modal]')?.previousElementSibling;
                if (modal && modal.classList.contains('modal')) {
                    closeModal(modal.id);
                }
            }
        });

        document.addEventListener('click', handleModalBackdropClick);
        document.addEventListener('keydown', handleModalKeyboard);

        // Set modal aria attributes
        document.querySelectorAll('.modal').forEach(modal => {
            modal.setAttribute('role', 'dialog');
            modal.setAttribute('aria-modal', 'true');
            modal.setAttribute('aria-hidden', 'true');
        });

        // Add form submission handlers for equipment modal
        const equipmentForm = document.querySelector('#equipment-modal form');
        if (equipmentForm) {
            equipmentForm.addEventListener('submit', (event) => {
                const submitButton = equipmentForm.querySelector('button[type="submit"]');
                if (submitButton) {
                    setButtonLoading(submitButton, true);
                }
            });
        }
    }

    // Export functions to global scope for inline event handlers
    window.updateAccess = updateAccess;
    window.showEquipmentModal = showEquipmentModal;
    window.closeEquipmentModal = closeEquipmentModal;
    window.confirmDeleteEquipment = confirmDeleteEquipment;
    window.resetPassword = resetPassword;
    window.confirmRemoveUser = confirmRemoveUser;
    window.showReportModal = showReportModal;
    window.closeReportModal = closeReportModal;
    window.downloadReport = downloadReport;

    // Initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
