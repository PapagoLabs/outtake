(function () {
	var next = document.body.dataset.authNext || '/';
	if (window.opener) {
		window.opener.location = next;
		window.close();
		return;
	}
	window.location = next;
})();
