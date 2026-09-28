(function () {
	var startEl = document.getElementById('startTime');
	var endEl = document.getElementById('endTime');
	var durEl = document.getElementById('duration');
	var label = document.getElementById('duration-label');
	var warning = document.getElementById('duration-warning');
	var maxDur = parseInt(document.getElementById('clip-form-config').getAttribute('data-max-dur'), 10) || 600;
	function pad2(n) { return String(n).padStart(2, '0'); }
	function formatTimecode(sec) {
		if (!isFinite(sec) || sec < 0) { sec = 0; }
		var ms = Math.round(sec * 1000);
		var whole = Math.floor(ms / 1000);
		var frac = ms % 1000;
		var h = Math.floor(whole / 3600);
		var m = Math.floor((whole % 3600) / 60);
		var s = whole % 60;
		return pad2(h) + ':' + pad2(m) + ':' + pad2(s) + '.' + String(frac).padStart(3, '0');
	}
	function parseTimecode(value) {
		var parts = String(value).trim().split(':');
		if (parts.length === 1) { return parseFloat(parts[0]) || 0; }
		if (parts.length === 2) { return (parseInt(parts[0], 10) || 0) * 60 + (parseFloat(parts[1]) || 0); }
		return (parseInt(parts[0], 10) || 0) * 3600 + (parseInt(parts[1], 10) || 0) * 60 + (parseFloat(parts[2]) || 0);
	}
	function syncDuration() {
		var start = parseTimecode(startEl.value);
		var end = parseTimecode(endEl.value);
		var dur = Math.max(0, end - start);
		if (dur > maxDur) {
			dur = maxDur;
			if (warning) { warning.classList.remove('hidden'); }
		} else if (warning) {
			warning.classList.add('hidden');
		}
		durEl.value = dur.toFixed(3);
		label.textContent = formatTimecode(dur);
	}
	document.addEventListener('click', function (event) {
		var startBtn = event.target.closest('.js-mark-start');
		var endBtn = event.target.closest('.js-mark-end');
		if (startBtn) {
			startEl.value = formatTimecode(parseFloat(startBtn.getAttribute('data-offset')));
			var start = parseTimecode(startEl.value);
			var end = parseTimecode(endEl.value);
			if (end <= start) {
				var dur = parseFloat(durEl.value) || 0;
				if (dur <= 0) { dur = 10; }
				endEl.value = formatTimecode(start + dur);
			}
			syncDuration();
		}
		if (endBtn) {
			endEl.value = formatTimecode(parseFloat(endBtn.getAttribute('data-offset')));
			syncDuration();
		}
	});
	startEl.addEventListener('input', syncDuration);
	endEl.addEventListener('input', syncDuration);
	syncDuration();

	// The preview renders in the background, so the file this page was
	// redirected to may not exist yet. Poll until the server publishes it, then
	// hand the URL to the player rather than pointing at it up front.
	var videoEl = document.getElementById('preview-video');
	var statusEl = document.getElementById('preview-status');
	if (!videoEl || !statusEl) { return; }

	var previewId = videoEl.getAttribute('data-preview-id');
	var attempts = 0;
	var maxAttempts = 900; // roughly five minutes at the poll interval below
	var stopped = false;

	document.addEventListener('htmx:beforeCleanup', function () {
		stopped = true;
	});

	function pollPreview() {
		if (stopped) { return; }
		fetch('/api/clips/preview/' + encodeURIComponent(previewId))
			.then(function (response) {
				if (!response.ok) { throw new Error('status ' + response.status); }

				return response.json();
			})
			.then(function (state) {
				if (state.status === 'completed' && state.url) {
					statusEl.textContent = '';
					statusEl.classList.add('hidden');
					videoEl.src = state.url;
					videoEl.load();

					return;
				}
				if (state.status === 'failed' || state.status === 'canceled') {
					statusEl.textContent = state.error || 'Preview was not rendered.';

					return;
				}
				if (state.progress) {
					statusEl.textContent = 'Rendering preview… ' + state.progress + '%';
				}
				attempts++;
				if (attempts < maxAttempts) {
					window.setTimeout(pollPreview, 1000);
				}
			})
			.catch(function () {
				attempts++;
				if (attempts < maxAttempts) {
					window.setTimeout(pollPreview, 2000);
				}
			});
	}

	pollPreview();
})();
