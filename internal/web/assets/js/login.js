(function () {
	document.getElementById('plex-login').addEventListener('click', async function () {
		var errorEl = document.getElementById('plex-error');
		var statusEl = document.getElementById('auth-status');
		errorEl.classList.add('hidden');
		errorEl.textContent = '';
		try {
			var res = await fetch('/api/auth/login', {
				method: 'POST',
				headers: { 'Accept': 'application/json' }
			});
			var data = await res.json();
			if (!res.ok || !data.authUrl) {
				errorEl.textContent = data.message || 'Failed to start Plex sign-in.';
				errorEl.classList.remove('hidden');
				return;
			}
			window.open(data.authUrl, 'plex-auth', 'popup=yes,width=800,height=740');
			statusEl.classList.remove('hidden');
		} catch (err) {
			errorEl.textContent = 'Failed to start Plex sign-in.';
			errorEl.classList.remove('hidden');
		}
	});
})();
