(() => {
  'use strict';
  document.querySelectorAll('[data-cast-editor]').forEach(editor => {
    const rows = editor.querySelector('[data-cast-rows]');
    const add = editor.querySelector('[data-add-cast]');
    const prototype = rows.firstElementChild.cloneNode(true);
    let pickerCount = 0;
    const setupPicker = row => {
      const select = row.querySelector('select');
      const options = Array.from(select.options).filter(option => option.value);
      const picker = document.createElement('div');
      picker.className = 'cast-picker';
      const search = document.createElement('input');
      search.type = 'search';
      search.placeholder = 'พิมพ์ชื่อนักแสดงเพื่อค้นหา…';
      search.autocomplete = 'off';
      search.setAttribute('role', 'combobox');
      search.setAttribute('aria-autocomplete', 'list');
      search.setAttribute('aria-expanded', 'false');
      const results = document.createElement('div');
      results.className = 'cast-picker-results';
      results.id = `cast-options-${++pickerCount}`;
      results.setAttribute('role', 'listbox');
      results.hidden = true;
      search.setAttribute('aria-controls', results.id);
      search.value = select.value ? select.selectedOptions[0].textContent : '';
      select.hidden = true;
      select.before(picker);
      picker.append(search, results);
      let active = -1;
      const close = () => {
        results.hidden = true;
        search.setAttribute('aria-expanded', 'false');
        search.removeAttribute('aria-activedescendant');
        active = -1;
      };
      const render = () => {
        results.replaceChildren();
        active = -1;
        search.removeAttribute('aria-activedescendant');
        const query = search.value.trim().toLocaleLowerCase();
        const matches = options.filter(option => option.textContent.toLocaleLowerCase().includes(query));
        matches.slice(0, 30).forEach((option, index) => {
          const item = document.createElement('div');
          item.id = `${results.id}-${index}`;
          item.setAttribute('role', 'option');
          item.setAttribute('aria-selected', 'false');
          item.textContent = option.textContent;
          item.addEventListener('mousedown', event => event.preventDefault());
          item.addEventListener('click', () => {
            select.value = option.value;
            search.value = option.textContent;
            search.setCustomValidity('');
            select.dispatchEvent(new Event('change', {bubbles: true}));
            close();
          });
          results.append(item);
        });
        if (!matches.length) {
          const empty = document.createElement('p');
          empty.textContent = 'ไม่พบชื่อนี้ ลองค้นหาหรือเพิ่มนักแสดงก่อน';
          results.append(empty);
        }
        results.hidden = false;
        search.setAttribute('aria-expanded', 'true');
      };
      search.addEventListener('focus', render);
      search.addEventListener('input', () => {
        select.value = '';
        search.setCustomValidity(search.value.trim() ? 'กรุณาเลือกนักแสดงจากผลการค้นหา' : '');
        render();
      });
      search.addEventListener('blur', close);
      search.addEventListener('keydown', event => {
        if (event.key === 'Escape') { close(); return; }
        if (event.key === 'Enter') {
          event.preventDefault();
          const items = results.querySelectorAll('[role="option"]');
          if (!results.hidden && items.length) items[active < 0 ? 0 : active].click();
          return;
        }
        if (!['ArrowDown', 'ArrowUp'].includes(event.key)) return;
        event.preventDefault();
        if (results.hidden) render();
        const items = results.querySelectorAll('[role="option"]');
        if (!items.length) return;
        active = (active + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length;
        items.forEach((item, index) => item.setAttribute('aria-selected', String(index === active)));
        search.setAttribute('aria-activedescendant', items[active].id);
        items[active].scrollIntoView({block: 'nearest'});
      });
    };
    Array.from(rows.children).forEach(setupPicker);
    const update = () => {
      add.disabled = rows.children.length >= 100;
      rows.querySelectorAll('[data-remove-cast]').forEach(button => { button.hidden = false; });
    };
    add.hidden = false;
    add.addEventListener('click', () => {
      if (rows.children.length >= 100) return;
      const row = prototype.cloneNode(true);
      row.querySelector('select').value = '';
      row.querySelector('input').value = '';
      rows.append(row);
      setupPicker(row);
      update();
      row.querySelector('input[type="search"]').focus();
    });
    rows.addEventListener('click', event => {
      const button = event.target.closest('[data-remove-cast]');
      if (!button) return;
      const row = button.closest('[data-cast-row]');
      if (rows.children.length === 1) {
        row.querySelector('select').value = '';
        row.querySelectorAll('input').forEach(input => {
          input.value = '';
          input.setCustomValidity('');
        });
      } else row.remove();
      update();
    });
    update();
  });
  const browse = document.querySelector('.tmdb-browse-form');
  if (browse) {
    const category = browse.querySelector('[name="category"]');
    const query = browse.querySelector('[name="q"]');
    query.addEventListener('input', () => {
      if (query.value.trim()) category.value = 'search';
    });
    category.addEventListener('change', () => {
      if (category.value !== 'search') query.value = '';
    });
  }
  const form = document.querySelector('[data-bulk-import]');
  if (form) {
    const choices = [...form.querySelectorAll('input[name="tmdb_id"]')];
    const all = form.querySelector('[data-select-all]');
    const button = form.querySelector('[data-import-selected]');
    const count = form.querySelector('[data-selection-count]');
    const progress = form.querySelector('[data-import-progress]');
    let busy = false;
    button.hidden = false;
    const update = () => {
      choices.forEach(input => {
        const card = input.closest('[data-import-item]');
        card.classList.toggle('is-selected', input.checked);
        card.classList.toggle('is-selectable', !input.disabled);
      });
      const available = choices.filter(input => !input.disabled);
      const selected = available.filter(input => input.checked);
      count.textContent = `${selected.length} selected`;
      button.disabled = busy || !selected.length;
      all.disabled = busy || !available.length;
      all.checked = available.length > 0 && selected.length === available.length;
      all.indeterminate = selected.length > 0 && selected.length < available.length;
    };
    all.addEventListener('change', () => {
      choices.filter(input => !input.disabled).forEach(input => { input.checked = all.checked; });
      update();
    });
    choices.forEach(input => {
      input.addEventListener('change', update);
      const card = input.closest('[data-import-item]');
      card.addEventListener('click', event => {
        if (busy || input.disabled) return;
        // Let native controls handle their own clicks so labels never toggle twice.
        if (event.target.closest('input, label, a, button, select, textarea')) return;
        input.click();
      });
    });
    window.addEventListener('beforeunload', event => {
      if (busy) { event.preventDefault(); event.returnValue = ''; }
    });
    form.addEventListener('submit', async event => {
      event.preventDefault();
      if (busy) return;
      const selected = choices.filter(input => input.checked && !input.disabled);
      if (!selected.length) return;
      busy = true;
      choices.forEach(input => { input.dataset.wasDisabled = String(input.disabled); input.disabled = true; });
      update();
      let imported = 0, skipped = 0, failed = 0, processed = 0;
      const castResults = new Map();
      for (const input of selected) {
        const status = input.closest('[data-import-item]').querySelector('[data-import-status]');
        progress.textContent = `Importing ${processed + 1} of ${selected.length}…`;
        status.textContent = 'Importing…';
        try {
          const response = await fetch(form.action, {
            method: 'POST', credentials: 'same-origin',
            headers: {'Content-Type': 'application/x-www-form-urlencoded', 'Accept': 'application/json'},
            body: new URLSearchParams({csrf_token: form.elements.csrf_token.value, tmdb_id: input.value})
          });
          if (response.redirected || !response.headers.get('content-type')?.includes('application/json')) {
            status.textContent = 'Session or request rejected. Refresh and sign in again.';
            progress.textContent = `Stopped. ${imported} imported, ${skipped} skipped. Refresh and try again.`;
            break;
          }
          const result = await response.json();
          if (response.ok && (result.status === 'imported' || result.status === 'skipped')) {
            if (result.status === 'imported') imported++; else skipped++;
            let castFailed = 0, noBio = 0, checked = 0;
            const cast = result.cast || [];
            for (const person of cast) {
              progress.textContent = `Movie ${processed + 1}/${selected.length}: checking cast ${checked + 1}/${cast.length} — ${person.name}`;
              status.textContent = `Movie saved. Cast ${checked}/${cast.length} complete…`;
              if (!person.needs_details) { checked++; continue; }
              let details = castResults.get(person.person_id);
              if (!details) {
                try {
                  const castResponse = await fetch(`/movies/${Number(result.movie_id)}/cast/${Number(person.person_id)}/import`, {
                    method: 'POST', credentials: 'same-origin',
                    headers: {'Content-Type': 'application/x-www-form-urlencoded', 'Accept': 'application/json'},
                    body: new URLSearchParams({csrf_token: form.elements.csrf_token.value})
                  });
                  if (castResponse.redirected || !castResponse.headers.get('content-type')?.includes('application/json')) {
                    const error = new Error('Session or request rejected. Refresh and sign in again.');
                    error.authLost = true; throw error;
                  }
                  details = await castResponse.json();
                  if (!castResponse.ok || !['ready', 'updated', 'no_bio'].includes(details.status)) {
                    castFailed++; checked++; continue;
                  }
                  castResults.set(person.person_id, details);
                } catch (error) {
                  if (error.authLost) throw error;
                  castFailed++; checked++; continue;
                }
              }
              if (details.status === 'no_bio') noBio++;
              checked++;
            }
            if (castFailed) {
              failed++;
              input.dataset.wasDisabled = 'false'; input.checked = true;
              status.textContent = `Movie saved; ${castFailed} cast member(s) failed. Select again to retry.`;
            } else {
              input.dataset.wasDisabled = 'true'; input.checked = false;
              status.textContent = `Movie and cast complete.${noBio ? ` TMDB has no biography for ${noBio} actor(s).` : ''}`;
            }
          } else { failed++; status.textContent = result.message || 'Failed. Select again to retry.'; }
        } catch (error) {
          if (error.authLost) {
            status.textContent = error.message;
            progress.textContent = 'Stopped. Saved data is kept. Refresh, sign in, and select again to continue.';
            break;
          }
          failed++; status.textContent = 'Connection interrupted. Retry to check/import this movie.';
        }
        processed++;
        progress.textContent = `${processed}/${selected.length} checked — ${imported} movies imported, ${skipped} already existed, ${failed} need retry.`;
      }
      busy = false;
      choices.forEach(input => { input.disabled = input.dataset.wasDisabled === 'true'; });
      update();
    });
    update();
  }

  document.querySelectorAll('[data-image-picker]').forEach(picker => {
    const input = picker.querySelector('[data-image-file]');
    const url = picker.querySelector('[data-image-url]');
    const drop = picker.querySelector('[data-image-drop]');
    const preview = picker.querySelector('[data-image-preview]');
    const status = picker.querySelector('[data-image-status]');
    let objectURL;
    const clear = document.createElement('button');
    clear.type = 'button'; clear.className = 'button subtle compact';
    clear.textContent = 'Remove selected file / use URL'; clear.hidden = true;
    status.after(clear);
    const update = () => {
      if (objectURL) { URL.revokeObjectURL(objectURL); objectURL = undefined; }
      input.setCustomValidity('');
      const file = input.files[0];
      clear.hidden = !file;
      if (file && (!['image/jpeg', 'image/png'].includes(file.type) || file.size > 5 * 1024 * 1024)) {
        input.setCustomValidity('Choose a JPG or PNG no larger than 5 MB.');
        status.textContent = input.validationMessage; preview.hidden = true; return;
      }
      if (file) {
        objectURL = URL.createObjectURL(file); preview.src = objectURL;
        status.textContent = `${file.name} — will replace the URL when you save.`;
        preview.hidden = false;
      } else {
        const value = url.value.trim();
        const valid = /^(https?:\/\/|\/(?!\/))/.test(value);
        preview.hidden = !valid;
        if (valid) preview.src = value; else preview.removeAttribute('src');
        status.textContent = value ? 'Using image URL.' : '';
      }
    };
    preview.addEventListener('error', () => { preview.hidden = true; status.textContent = 'Preview unavailable. Check the image link or file.'; });
    input.addEventListener('change', update);
    url.addEventListener('change', update);
    clear.addEventListener('click', () => { input.value = ''; update(); });
    ['dragenter', 'dragover'].forEach(name => drop.addEventListener(name, event => { event.preventDefault(); drop.classList.add('drag-over'); }));
    drop.addEventListener('dragleave', () => drop.classList.remove('drag-over'));
    drop.addEventListener('drop', event => {
      event.preventDefault(); drop.classList.remove('drag-over');
      if (event.dataTransfer.files.length !== 1) { status.textContent = 'Drop one image at a time.'; return; }
      input.files = event.dataTransfer.files; update();
    });
    update();
  });
})();
