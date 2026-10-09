(function () {
	var next = document.body.dataset.authNext || '/';
	if (window.opener) {
		window.opener.location = next;
		window.close();
		return;
	}
	// A popup cut off from its opener still closes, and the login page then
	// moves on through its own status poll. A page that is not a popup cannot
	// close, so it carries on to the next page itself.
	window.close();
	window.setTimeout(function () {
		window.location = next;
	}, 250);
})();
