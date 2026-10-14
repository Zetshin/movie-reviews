(() => {
  const form = document.querySelector('.avatar-form');
  const dialog = document.querySelector('.avatar-dialog');
  if (!form || !dialog || !window.createImageBitmap || !window.DataTransfer || !dialog.showModal) return;

  const trigger = document.querySelector('[data-avatar-trigger]');
  const input = form.querySelector('input[type="file"]');
  const error = form.querySelector('[data-avatar-error]');
  const dialogError = dialog.querySelector('[data-avatar-dialog-error]');
  const canvas = dialog.querySelector('canvas');
  const ctx = canvas.getContext('2d');
  const zoom = dialog.querySelector('input[type="range"]');
  const save = dialog.querySelector('[data-avatar-save]');
  let image = null;
  let x = 0, y = 0, scale = 1, drag = null, revision = 0;
  const size = canvas.width;
  // Match the circular mask's 12% inset in site.css.
  const cropInset = size * 0.12;
  const cropSize = size - cropInset * 2;
  const showError = (target, message) => { target.textContent = message; target.hidden = !message; };

  function draw() {
    if (!image) return;
    const width = image.width * scale, height = image.height * scale;
    x = Math.max(size - width, Math.min(0, x));
    y = Math.max(size - height, Math.min(0, y));
    ctx.fillStyle = '#18212b';
    ctx.fillRect(0, 0, size, size);
    ctx.drawImage(image, x, y, width, height);
  }

  trigger.hidden = false;
  form.classList.add('avatar-editor-ready');
  trigger.addEventListener('click', () => { input.value = ''; input.click(); });
  input.addEventListener('change', async () => {
    const currentRevision = ++revision;
    const file = input.files[0];
    showError(error, '');
    if (!file) return;
    if (!['image/jpeg', 'image/png'].includes(file.type) || file.size > 2 * 1024 * 1024) {
      input.value = '';
      showError(error, 'Choose a JPG or PNG image no larger than 2 MB.');
      return;
    }
    try {
      const bitmap = await createImageBitmap(file);
      if (currentRevision !== revision) { bitmap.close(); return; }
      if (bitmap.width * bitmap.height > 16000000) {
        bitmap.close();
        throw new Error('Choose an image with no more than 16 million pixels.');
      }
      if (image) image.close();
      image = bitmap;
      zoom.value = '1';
      scale = Math.max(size / image.width, size / image.height);
      x = (size - image.width * scale) / 2;
      y = (size - image.height * scale) / 2;
      showError(dialogError, '');
      draw();
      dialog.showModal();
      canvas.focus();
    } catch (err) {
      input.value = '';
      showError(error, err.message === 'Choose an image with no more than 16 million pixels.' ? err.message : 'Could not read this image. Try another JPG or PNG.');
    }
  });

  zoom.addEventListener('input', () => {
    if (!image) return;
    const nextScale = Math.max(size / image.width, size / image.height) * Number(zoom.value);
    x = size / 2 - (size / 2 - x) * nextScale / scale;
    y = size / 2 - (size / 2 - y) * nextScale / scale;
    scale = nextScale;
    draw();
  });
  dialog.querySelectorAll('[data-avatar-zoom]').forEach(button => {
    button.addEventListener('click', () => {
      if (save.disabled) return;
      const step = button.dataset.avatarZoom === 'in' ? 0.1 : -0.1;
      zoom.value = String(Math.max(1, Math.min(3, Number(zoom.value) + step)));
      zoom.dispatchEvent(new Event('input'));
    });
  });
  canvas.addEventListener('pointerdown', event => {
    if (event.button !== 0 || !image || save.disabled) return;
    drag = {x: event.clientX, y: event.clientY};
    canvas.setPointerCapture(event.pointerId);
  });
  canvas.addEventListener('pointermove', event => {
    if (!drag) return;
    const ratio = size / canvas.getBoundingClientRect().width;
    x += (event.clientX - drag.x) * ratio;
    y += (event.clientY - drag.y) * ratio;
    drag = {x: event.clientX, y: event.clientY};
    draw();
  });
  ['pointerup', 'pointercancel', 'lostpointercapture'].forEach(type => canvas.addEventListener(type, () => { drag = null; }));
  canvas.addEventListener('keydown', event => {
    const moves = {ArrowLeft: [-10, 0], ArrowRight: [10, 0], ArrowUp: [0, -10], ArrowDown: [0, 10]};
    if (!moves[event.key] || save.disabled) return;
    event.preventDefault();
    x += moves[event.key][0]; y += moves[event.key][1]; draw();
  });
  dialog.querySelectorAll('[data-avatar-cancel]').forEach(button => button.addEventListener('click', () => { if (!save.disabled) dialog.close(); }));
  dialog.addEventListener('cancel', event => { if (save.disabled) event.preventDefault(); });
  dialog.addEventListener('close', () => {
    ++revision;
    drag = null;
    if (image) { image.close(); image = null; }
    input.value = '';
    trigger.focus();
  });
  save.addEventListener('click', () => {
    if (!image || save.disabled) return;
    save.disabled = true;
    save.textContent = 'Saving…';
    showError(dialogError, '');
    const output = document.createElement('canvas');
    output.width = output.height = 256;
    // Save the square enclosing the visible circle, so navbar framing matches.
    output.getContext('2d').drawImage(canvas, cropInset, cropInset, cropSize, cropSize, 0, 0, 256, 256);
    output.toBlob(blob => {
      try {
        if (!blob) throw new Error('Could not prepare this photo. Please try again.');
        const transfer = new DataTransfer();
        transfer.items.add(new File([blob], 'avatar.png', {type: 'image/png'}));
        input.files = transfer.files;
        // Submit the adjusted image to the existing protected upload route.
        form.requestSubmit();
      } catch (err) {
        showError(dialogError, 'Could not save this photo. Please try again.');
        save.disabled = false;
        save.textContent = 'Save photo';
      }
    }, 'image/png');
  });
})();
