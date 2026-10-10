(function () {
	var pollIntervalMs = 2000;
	// fallbackLifetimeMs is how long a PIN is waited on when Plex did not say
	// how long it lasts. Strong PINs last half an hour.
	var fallbackLifetimeMs = 30 * 60 * 1000;
	// attempt counts sign-in clicks. Every asynchronous step checks it before
	// touching the page, so an earlier attempt's late answer cannot redirect
	// the popup or keep polling once a newer attempt has started.
	var attempt = 0;
	var pollTimer = null;
	var expiryTimer = null;
	var pending = null;

	function showError(errorEl, message) {
		errorEl.textContent = message;
		errorEl.classList.remove('hidden');
	}

	// stopPolling ends every part of a sign-in attempt's polling: the next
	// poll, the expiry, and any status request still in flight.
	function stopPolling() {
		if (pollTimer !== null) {
			window.clearTimeout(pollTimer);
			pollTimer = null;
		}
		if (expiryTimer !== null) {
			window.clearTimeout(expiryTimer);
			expiryTimer = null;
		}
		if (pending !== null) {
			pending.abort();
			pending = null;
		}
	}

	// pollStatus asks the server whether Plex has authorized the PIN, until it
	// has. The expiry timer ends it when the PIN lapses, even mid-request.
	function pollStatus(id, statusEl, errorEl) {
		if (id !== attempt) {
			return;
		}
		pollTimer = null;
		pending = new AbortController();
		var signal = pending.signal;

		function retry() {
			if (id !== attempt || signal.aborted) {
				return;
			}
			pending = null;
			pollTimer = window.setTimeout(function () {
				pollStatus(id, statusEl, errorEl);
			}, pollIntervalMs);
		}

		fetch(statusEl.getAttribute('data-status-url'), {
			headers: { 'Accept': 'text/plain' },
			signal: signal
		})
			.then(function (res) {
				var next = res.headers.get('HX-Redirect');
				return res.text().then(function (text) {
					if (id !== attempt || signal.aborted) {
						return;
					}
					if (text) {
						statusEl.textContent = text;
					}
					if (next) {
						stopPolling();
						window.location = next;
						return;
					}
					retry();
				});
			})
			.catch(retry);
	}

	// startPolling polls until the PIN is authorized or lapses.
	function startPolling(id, statusEl, errorEl, lifetimeMs) {
		stopPolling();
		expiryTimer = window.setTimeout(function () {
			if (id !== attempt) {
				return;
			}
			stopPolling();
			statusEl.classList.add('hidden');
			showError(errorEl, 'The Plex login expired. Try again.');
		}, lifetimeMs);
		pollStatus(id, statusEl, errorEl);
	}

	document.getElementById('plex-login').addEventListener('click', async function () {
		var errorEl = document.getElementById('plex-error');
		var statusEl = document.getElementById('auth-status');
		var id = ++attempt;
		stopPolling();
		errorEl.classList.add('hidden');
		errorEl.textContent = '';
		var popup = window.open('about:blank', 'plex-auth', 'popup=yes,width=800,height=740');
		if (!popup) {
			showError(errorEl, 'Couldn\'t open the Plex login. Allow pop-ups for this site and try again.');
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
			if (id !== attempt) {
				return;
			}
			if (!res.ok || !data.authUrl) {
				popup.close();
				showError(errorEl, data.message || 'Couldn\'t start the Plex login');
				return;
			}
			popup.location.href = data.authUrl;
			statusEl.classList.remove('hidden');
			var lifetimeMs = data.expiresIn > 0 ? data.expiresIn * 1000 : fallbackLifetimeMs;
			startPolling(id, statusEl, errorEl, lifetimeMs);
		} catch (err) {
			if (id !== attempt) {
				return;
			}
			popup.close();
			showError(errorEl, 'Couldn\'t start the Plex login');
		}
	});
})();
