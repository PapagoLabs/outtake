(function () {
	try {
		var stored = window.localStorage.getItem('outtake-theme');
		if (stored === 'light') {
			document.documentElement.classList.remove('dark');
		} else if (stored === 'dark') {
			document.documentElement.classList.add('dark');
		}
	} catch (err) {
		return;
	}
})();
