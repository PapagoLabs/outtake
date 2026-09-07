(function () {
	function paletteList() {
		var raw = document.documentElement.getAttribute('data-palettes');
		if (!raw) {
			return ['plex'];
		}

		return raw.split(',');
	}

	try {
		var stored = window.localStorage.getItem('outtake-theme');
		if (stored === 'light') {
			document.documentElement.classList.remove('dark');
		} else if (stored === 'dark') {
			document.documentElement.classList.add('dark');
		}

		var palettes = paletteList();
		var palette = window.localStorage.getItem('outtake-palette');
		if (palettes.indexOf(palette) === -1) {
			palette = 'plex';
		}
		document.documentElement.setAttribute('data-palette', palette);
	} catch (err) {
		return;
	}
})();
