var navLinks = document.querySelectorAll("nav a");
// Native details menus also work without JavaScript; close on outside click or Escape.
document.addEventListener('click', event => {
  document.querySelectorAll('.admin-actions[open]').forEach(menu => {
    if (!menu.contains(event.target)) menu.open = false;
  });
});
document.addEventListener('keydown', event => {
  if (event.key !== 'Escape') return;
  document.querySelectorAll('.admin-actions[open]').forEach(menu => {
    const containedFocus = menu.contains(document.activeElement);
    menu.open = false;
    if (containedFocus) menu.querySelector('summary').focus();
  });
});
// Scroll movie credits without adding height to the page.
document.querySelectorAll('[data-cast-carousel]').forEach(carousel => {
  const track = carousel.querySelector('.cast-grid');
  const previous = carousel.querySelector('[data-cast-direction="-1"]');
  const next = carousel.querySelector('[data-cast-direction="1"]');
  const update = () => {
    const maximum = track.scrollWidth - track.clientWidth;
    previous.hidden = next.hidden = maximum <= 2;
    previous.disabled = track.scrollLeft <= 2;
    next.disabled = track.scrollLeft >= maximum - 2;
  };
  [previous, next].forEach(button => {
    button.addEventListener('click', () => {
      track.scrollBy({
        left: Number(button.dataset.castDirection) * Math.max(160, track.clientWidth * 0.8),
        behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth'
      });
    });
  });
  track.addEventListener('scroll', update, {passive: true});
  window.addEventListener('resize', update);
  if ('ResizeObserver' in window) new ResizeObserver(update).observe(track);
  update();
});
document.querySelectorAll('.review-sort select').forEach(select => {
  select.addEventListener('change', () => select.form.requestSubmit());
});
for (var i = 0; i < navLinks.length; i++) {
	var link = navLinks[i]
	if (link.getAttribute('href') == window.location.pathname) {
		link.classList.add("live");
		break;
	}
}
(() => {
  const picker = document.querySelector('.star-picker');
  if (!picker) return;
  const output = document.querySelector('.selected-rating');
  picker.addEventListener('change', event => {
    if (event.target.name === 'rating') output.textContent = `${event.target.value} / 5 stars`;
  });
})();
