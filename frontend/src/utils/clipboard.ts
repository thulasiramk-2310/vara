// copyText puts text on the clipboard and says whether it worked.
//
// The modern clipboard API exists only on secure pages (HTTPS or localhost), and
// a Hub on a company network is often plain HTTP, where `navigator.clipboard` is
// undefined. It can also be refused by a permission prompt. So this tries the
// modern way, falls back to the older selection-based way, and reports honestly,
// instead of throwing an unhandled error and claiming success.
export async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // fall through to the older method
  }
  try {
    const area = document.createElement('textarea');
    area.value = text;
    area.setAttribute('readonly', '');
    area.style.cssText = 'position:fixed;top:0;left:0;opacity:0;pointer-events:none';
    document.body.appendChild(area);
    area.select();
    const ok = document.execCommand('copy');
    document.body.removeChild(area);
    return ok;
  } catch {
    return false;
  }
}
