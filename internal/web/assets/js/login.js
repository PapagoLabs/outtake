(function () {
	document.getElementById('plex-login').addEventListener('click', async function () {
		var errorEl = document.getElementById('plex-error');
		var statusEl = document.getElementById('auth-status');
		errorEl.classList.add('hidden');
		errorEl.textContent = '';
		var popup = window.open('about:blank', 'plex-auth', 'popup=yes,width=800,height=740');
		if (!popup) {
			errorEl.textContent = 'Failed to start Plex sign-in.';
			errorEl.classList.remove('hidden');
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
				errorEl.textContent = data.message || 'Failed to start Plex sign-in.';
				errorEl.classList.remove('hidden');
				return;
			}
			popup.location.href = data.authUrl;
			statusEl.classList.remove('hidden');
		} catch (err) {
			popup.close();
			errorEl.textContent = 'Failed to start Plex sign-in.';
			errorEl.classList.remove('hidden');
		}
	});
})();
