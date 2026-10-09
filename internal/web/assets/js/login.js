(function () {
	var pollIntervalMs = 2000;
	// fallbackLifetimeMs is how long a PIN is waited on when Plex did not say
	// how long it lasts. Strong PINs last half an hour.
	var fallbackLifetimeMs = 30 * 60 * 1000;
	var pollTimer = null;

	function showError(errorEl, message) {
		errorEl.textContent = message;
		errorEl.classList.remove('hidden');
	}

	function stopPolling() {
		if (pollTimer !== null) {
			window.clearTimeout(pollTimer);
			pollTimer = null;
		}
	}

	// pollStatus asks the server whether Plex has authorized the PIN, until it
	// has, or until the PIN lapses. It starts only once the popup is open, so
	// a page nobody signs in from asks nothing.
	function pollStatus(statusEl, errorEl, deadline) {
		stopPolling();
		if (Date.now() >= deadline) {
			statusEl.classList.add('hidden');
			showError(errorEl, 'The Plex sign-in expired. Sign in again.');
			return;
		}
		fetch(statusEl.getAttribute('data-status-url'), { headers: { 'Accept': 'text/plain' } })
			.then(function (res) {
				var next = res.headers.get('HX-Redirect');
				return res.text().then(function (text) {
					if (text) {
						statusEl.textContent = text;
					}
					if (next) {
						window.location = next;
						return;
					}
					pollTimer = window.setTimeout(function () {
						pollStatus(statusEl, errorEl, deadline);
					}, pollIntervalMs);
				});
			})
			.catch(function () {
				pollTimer = window.setTimeout(function () {
					pollStatus(statusEl, errorEl, deadline);
				}, pollIntervalMs);
			});
	}

	document.getElementById('plex-login').addEventListener('click', async function () {
		var errorEl = document.getElementById('plex-error');
		var statusEl = document.getElementById('auth-status');
		stopPolling();
		errorEl.classList.add('hidden');
		errorEl.textContent = '';
		var popup = window.open('about:blank', 'plex-auth', 'popup=yes,width=800,height=740');
		if (!popup) {
			showError(errorEl, 'Failed to start Plex sign-in.');
			return;
		}
		try {
			var csrf = document.querySelector('meta[name="csrf-token"]');
			var headers = { 'Accept': 'application/json' };
			if (csrf && csrf.content) {
				headers['X-Csrf-Token'] = csrf.content;
			}
			var res = await fetch('/api/auth/login', {
				method: 'POST',
				headers: headers
			});
			var data = await res.json();
			if (!res.ok || !data.authUrl) {
				popup.close();
				showError(errorEl, data.message || 'Failed to start Plex sign-in.');
				return;
			}
			popup.location.href = data.authUrl;
			statusEl.classList.remove('hidden');
			var lifetimeMs = data.expiresIn > 0 ? data.expiresIn * 1000 : fallbackLifetimeMs;
			pollStatus(statusEl, errorEl, Date.now() + lifetimeMs);
		} catch (err) {
			popup.close();
			showError(errorEl, 'Failed to start Plex sign-in.');
		}
	});
})();
