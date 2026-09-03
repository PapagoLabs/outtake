(function () {
	var startEl = document.getElementById('startTime');
	var endEl = document.getElementById('endTime');
	var durEl = document.getElementById('duration');
	var label = document.getElementById('duration-label');
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
		if (dur > maxDur) { dur = maxDur; }
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
})();
