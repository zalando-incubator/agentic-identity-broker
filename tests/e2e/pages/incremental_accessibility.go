package pages

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mxschmitt/playwright-go"
)

const incrementalContrastAudit = `(selectors) => {
  const canvas = document.createElement('canvas');
  canvas.width = canvas.height = 1;
  const ctx = canvas.getContext('2d', {willReadFrequently: true});
  const rgba = (color) => {
    ctx.clearRect(0, 0, 1, 1);
    ctx.fillStyle = color;
    ctx.fillRect(0, 0, 1, 1);
    return Array.from(ctx.getImageData(0, 0, 1, 1).data).map((n, i) => i === 3 ? n / 255 : n);
  };
  const blend = (front, back) => front.slice(0, 3).map((n, i) => n * front[3] + back[i] * (1 - front[3])).concat(1);
  const background = (element) => {
    const ancestors = [];
    for (let node = element; node; node = node.parentElement) ancestors.unshift(node);
    return ancestors.reduce((color, node) => blend(rgba(getComputedStyle(node).backgroundColor), color), [255, 255, 255, 1]);
  };
  const luminance = (color) => color.slice(0, 3).map(n => {
    n /= 255;
    return n <= 0.04045 ? n / 12.92 : Math.pow((n + 0.055) / 1.055, 2.4);
  }).reduce((sum, n, i) => sum + n * [0.2126, 0.7152, 0.0722][i], 0);
  const ratio = (a, b) => (Math.max(luminance(a), luminance(b)) + 0.05) / (Math.min(luminance(a), luminance(b)) + 0.05);
  const failures = [];
  let measured = 0;
  let minimum = Infinity;
  for (const element of document.querySelectorAll(selectors)) {
    if (!element.getClientRects().length || element.closest('[disabled], [aria-disabled="true"]')) continue;
    const hasText = Array.from(element.childNodes).some(n => n.nodeType === Node.TEXT_NODE && n.textContent.trim());
    if (!hasText) continue;
    const style = getComputedStyle(element);
    const bg = background(element);
    const value = ratio(blend(rgba(style.color), bg), bg);
    measured++;
    minimum = Math.min(minimum, value);
    if (value < 4.5) failures.push(element.textContent.trim() + ': ' + value.toFixed(2));
    if (element.matches('button')) {
      const boundary = ratio(bg, background(element.parentElement));
      if (boundary < 3) failures.push(element.textContent.trim() + ' button boundary: ' + boundary.toFixed(2));
    }
  }
  return {measured, minimum, failures};
}`

func (p *Page) auditIncrementalContrast(ctx context.Context, selectors string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	result, err := p.page.Evaluate(incrementalContrastAudit, selectors)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	var values struct {
		Measured int
		Minimum  float64
		Failures []string
	}
	if err := json.Unmarshal(encoded, &values); err != nil {
		return "", err
	}
	if values.Measured == 0 {
		return "", fmt.Errorf("no visible feature text was measured: %v", result)
	}
	if len(values.Failures) > 0 {
		return "", fmt.Errorf("feature text contrast below 4.5:1: %v", result)
	}
	return fmt.Sprintf("%d text nodes, minimum contrast %.2f:1", values.Measured, values.Minimum), nil
}

func (cp *ConsentPage) AuditRequestedServiceContrast(ctx context.Context) (string, error) {
	return cp.auditIncrementalContrast(ctx, "[data-testid=requested-service-banner] *, [data-testid=requested-candidate-badge], #consent-save-reason")
}

func (sp *SessionsPage) AuditServiceDetailContrast(ctx context.Context) (string, error) {
	return sp.auditIncrementalContrast(ctx, "main h2, main h2 + p, [data-testid=service-session-status] *, main button")
}

func (p *Page) SetColorScheme(ctx context.Context, dark bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	scheme := playwright.ColorSchemeLight
	if dark {
		scheme = playwright.ColorSchemeDark
	}
	return p.page.EmulateMedia(playwright.PageEmulateMediaOptions{ColorScheme: scheme, ReducedMotion: playwright.ReducedMotionReduce})
}

func (p *Page) keyboardActivate(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	button := p.page.GetByRole("button", playwright.PageGetByRoleOptions{Name: name, Exact: playwright.Bool(true)}).First()
	if err := button.Focus(); err != nil {
		return err
	}
	if err := p.page.Keyboard().Press("Shift+Tab"); err != nil {
		return err
	}
	if err := p.page.Keyboard().Press("Tab"); err != nil {
		return err
	}
	focused, err := button.Evaluate(`element => {
      const style = getComputedStyle(element);
      const ring = style.getPropertyValue('--tw-ring-shadow').trim();
      return document.activeElement === element && element.matches(':focus-visible') &&
        style.boxShadow !== 'none' && ring !== '' && ring !== '0 0 #0000';
    }`, nil)
	if err != nil || focused != true {
		return fmt.Errorf("keyboard focus must have a visible indicator on %q: %v (%v)", name, focused, err)
	}
	return button.Press("Enter")
}

func (cp *ConsentPage) KeyboardLoginRequestedService(ctx context.Context, name string) error {
	return cp.keyboardActivate(ctx, "Log in to "+name)
}

func (sp *SessionsPage) KeyboardRefreshService(ctx context.Context) error {
	return sp.keyboardActivate(ctx, "Refresh")
}

func (cp *ConsentPage) VerifySaveDescription(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	button := cp.page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save", Exact: playwright.Bool(true)})
	disabled, err := button.IsDisabled()
	if err != nil || !disabled {
		return fmt.Errorf("save must be disabled for this check: %v", err)
	}
	description, err := button.GetAttribute("aria-describedby")
	if err != nil || description != "consent-save-reason" {
		return fmt.Errorf("save must reference its reason: %q (%v)", description, err)
	}
	reason, err := cp.page().Locator("#consent-save-reason").InnerText()
	if err != nil || reason == "" {
		return fmt.Errorf("save reason must be readable: %q (%v)", reason, err)
	}
	return nil
}
