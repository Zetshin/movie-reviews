(() => {
  'use strict';
  document.addEventListener('error', event => {
    if (event.target instanceof HTMLImageElement) event.target.classList.add('broken-image');
  }, true);
  const hero = document.querySelector('.hero');
  if (!hero) return;
  const slides = [...hero.querySelectorAll('[data-slide]')];
  const controls = hero.querySelector('.carousel-controls');
  if (slides.length < 2) { controls.hidden = true; return; }
  const dotsContainer = hero.querySelector('.slide-dots');
  const pause = hero.querySelector('[data-carousel="pause"]');
  const motion = window.matchMedia('(prefers-reduced-motion: reduce)');
  let current = 0;
  let paused = motion.matches;
  let timer;
  const dots = slides.map((slide, index) => {
    const dot = document.createElement('button');
    dot.type = 'button';
    dot.className = 'dot';
    dot.setAttribute('aria-label', `Show ${slide.getAttribute('aria-label')}`);
    dot.addEventListener('click', () => { show(index); schedule(); });
    dotsContainer.append(dot);
    return dot;
  });
  function show(index) {
    current = (index + slides.length) % slides.length;
    slides.forEach((slide, i) => { slide.hidden = i !== current; });
    dots.forEach((dot, i) => dot.setAttribute('aria-pressed', String(i === current)));
  }
  function schedule() {
    clearInterval(timer);
    pause.textContent = paused ? '▶' : 'Ⅱ';
    pause.setAttribute('aria-label', paused ? 'Start automatic slides' : 'Pause automatic slides');
    if (!paused && !document.hidden && !hero.matches(':hover') && !hero.contains(document.activeElement)) {
      timer = setInterval(() => show(current + 1), 6000);
    }
  }
  hero.querySelector('[data-carousel="previous"]').addEventListener('click', () => { show(current - 1); schedule(); });
  hero.querySelector('[data-carousel="next"]').addEventListener('click', () => { show(current + 1); schedule(); });
  pause.addEventListener('click', () => { paused = !paused; schedule(); });
  hero.addEventListener('mouseenter', schedule);
  hero.addEventListener('mouseleave', schedule);
  hero.addEventListener('focusin', schedule);
  hero.addEventListener('focusout', () => setTimeout(schedule, 0));
  document.addEventListener('visibilitychange', schedule);
  motion.addEventListener('change', () => { paused = motion.matches; schedule(); });
  show(0);
  schedule();
})();

// Search runs independently of the hero carousel (including single-slide pages).
(() => {
  const root = document.querySelector('[data-movie-search]');
  if (!root) return;
  const input = root.querySelector('input');
  const panel = root.querySelector('.search-dropdown');
  const status = root.querySelector('.search-status');
  const list = root.querySelector('ul');
  let timer, controller, version = 0, active = -1;
  function close() {
    clearTimeout(timer); controller?.abort(); version++;
    panel.hidden = true; input.setAttribute('aria-expanded', 'false');
    input.removeAttribute('aria-activedescendant'); active = -1;
  }
  function open(message) {
    panel.hidden = false; input.setAttribute('aria-expanded', 'true');
    status.textContent = message;
  }
  function select(index) {
    const options = [...list.children];
    active = index;
    options.forEach((option, i) => option.setAttribute('aria-selected', String(i === index)));
    if (options[index]) {
      input.setAttribute('aria-activedescendant', options[index].id);
      options[index].scrollIntoView({block: 'nearest'});
    }
  }
  function search() {
    clearTimeout(timer); controller?.abort();
    const request = ++version;
    const query = input.value.trim();
    list.replaceChildren(); active = -1; input.removeAttribute('aria-activedescendant');
    if (!query) { close(); return; }
    open('Searching…');
    timer = setTimeout(async () => {
      controller = new AbortController();
      try {
        const responses = await Promise.all(["movies", "persons"].map(type => fetch(`/${type}/search?q=${encodeURIComponent(query)}`, {signal: controller.signal})));
        if (responses.some(response => !response.ok)) throw new Error('Search failed');
        const [films, people] = await Promise.all(responses.map(response => response.json()));
        const movies = [...films.map(movie => ({...movie, kind: 'movie'})), ...people.map(person => ({id: person.id, title: person.name, poster_image: person.profile_image, kind: 'person'}))];
        if (request !== version) return;
        list.replaceChildren();
        for (const movie of movies) {
          const option = document.createElement('li');
          option.id = `search-${movie.kind}-${movie.id}`;
          option.setAttribute('role', 'option'); option.setAttribute('aria-selected', 'false');
          const link = document.createElement('a');
          link.href = `/${movie.kind === 'person' ? 'persons' : 'movies'}/${Number(movie.id)}`;
          const poster = document.createElement('span'); poster.className = 'search-poster'; poster.textContent = '★';
          if (movie.poster_image) {
            const url = new URL(movie.poster_image, location.origin);
            if (['http:', 'https:'].includes(url.protocol)) {
              const img = document.createElement('img'); img.src = url.href; img.alt = ''; poster.append(img);
            }
          }
          const copy = document.createElement('span');
          const title = document.createElement('strong'); title.textContent = movie.title;
          const meta = document.createElement('small'); meta.textContent = movie.kind === 'person' ? 'Person · View profile' : `Movie · ${movie.release_year || 'Year unavailable'} · ★ ${movie.avg_rating} / 5`;
          copy.append(title, meta); link.append(poster, copy); option.append(link); list.append(option);
        }
        open(movies.length ? `${movies.length} results` : 'No movies or people found');
      } catch (error) {
        if (error.name !== 'AbortError' && request === version) open('Could not search. Please try again.');
      }
    }, 300);
  }
  input.addEventListener('input', search);
  input.addEventListener('focus', () => { if (input.value.trim() && panel.hidden) search(); });
  input.addEventListener('keydown', event => {
    if (event.key === 'Escape') { close(); return; }
    if (panel.hidden || !list.children.length) return;
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      select(active < 0 ? (event.key === 'ArrowDown' ? 0 : list.children.length - 1) : (active + (event.key === 'ArrowDown' ? 1 : -1) + list.children.length) % list.children.length);
    } else if (event.key === 'Enter' && active >= 0) {
      event.preventDefault(); list.children[active].querySelector('a').click();
    }
  });
  document.addEventListener('pointerdown', event => { if (!root.contains(event.target)) close(); });
  root.addEventListener('focusout', event => { if (!root.contains(event.relatedTarget)) close(); });
})();
