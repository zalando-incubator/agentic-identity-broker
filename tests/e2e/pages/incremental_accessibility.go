package pages

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mxschmitt/playwright-go"
)

const incrementalContrastAudit = `async (selectors) => {
  await document.fonts.ready;
  await Promise.all(document.getAnimations()
    .filter(animation => animation.effect?.getTiming().iterations !== Infinity)
    .map(animation => animation.finished.catch(() => {})));
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
  let measuredUI = 0;
  let minimumUI = Infinity;
  for (const element of document.querySelectorAll(selectors)) {
    if (!element.getClientRects().length || element.closest('[disabled], [aria-disabled="true"]')) continue;
    const style = getComputedStyle(element);
    const bg = background(element);
    if (element.matches('button')) {
      const boundary = ratio(bg, background(element.parentElement));
      measuredUI++;
      minimumUI = Math.min(minimumUI, boundary);
      if (boundary < 3) failures.push((element.getAttribute('aria-label') || element.textContent.trim()) + ' button boundary: ' + boundary.toFixed(2));
    }
    const hasText = Array.from(element.childNodes).some(n => n.nodeType === Node.TEXT_NODE && n.textContent.trim());
    if (!hasText) continue;
    const value = ratio(blend(rgba(style.color), bg), bg);
    measured++;
    minimum = Math.min(minimum, value);
    if (value < 4.5) failures.push(element.textContent.trim() + ': ' + value.toFixed(2));
  }
  return {measured, minimum, measuredUI, minimumUI, failures};
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
		Measured   int
		Minimum    float64
		MeasuredUI int
		MinimumUI  float64
		Failures   []string
	}
	if err := json.Unmarshal(encoded, &values); err != nil {
		return "", err
	}
	if values.Measured == 0 || values.MeasuredUI == 0 {
		return "", fmt.Errorf("visible feature text and controls must both be measured: %v", result)
	}
	if len(values.Failures) > 0 {
		return "", fmt.Errorf("feature contrast below 4.5:1 text or 3:1 UI: %v", result)
	}
	return fmt.Sprintf("%d text nodes, minimum %.2f:1; %d UI controls, minimum %.2f:1", values.Measured, values.Minimum, values.MeasuredUI, values.MinimumUI), nil
}

func (cp *ConsentPage) AuditRequestedServiceContrast(ctx context.Context) (string, error) {
	return cp.auditIncrementalContrast(ctx, "[data-testid=requested-service-banner] *, [data-testid=requested-candidate-badge], #consent-save-reason, main button[title], [data-testid=permission-set-optional] button[role=switch]")
}

func (sp *SessionsPage) AuditServiceDetailContrast(ctx context.Context) (string, error) {
	return sp.auditIncrementalContrast(ctx, "main h2, main h2 + p, [data-testid=service-session-status] *, main section, main section *, main button:not([aria-label='Dismiss alert'])")
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

func (p *Page) verifyKeyboardOrder(ctx context.Context, controls ...playwright.Locator) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := controls[0].Focus(); err != nil {
		return err
	}
	if err := p.page.Keyboard().Press("Shift+Tab"); err != nil {
		return err
	}
	for index, control := range controls {
		found := false
		for range 40 {
			if err := p.page.Keyboard().Press("Tab"); err != nil {
				return err
			}
			focused, err := control.Evaluate(`element => document.activeElement === element`, nil)
			if err != nil {
				return err
			}
			if focused == true {
				found = true
				break
			}
			wrapped, err := controls[0].Evaluate(`element => document.activeElement === element`, nil)
			if err != nil || wrapped == true {
				return fmt.Errorf("keyboard order wrapped before control %d: %v", index, err)
			}
		}
		if !found {
			return fmt.Errorf("keyboard could not reach control %d in order", index)
		}
		visible, err := control.Evaluate(`element => {
		  const style = getComputedStyle(element);
		  const ring = style.getPropertyValue('--tw-ring-shadow').trim();
		  return element.matches(':focus-visible') && style.boxShadow !== 'none' && ring !== '' && ring !== '0 0 #0000';
		}`, nil)
		if err != nil || visible != true {
			return fmt.Errorf("control %d needs visible keyboard focus: %v (%v)", index, visible, err)
		}
	}
	return nil
}

func (cp *ConsentPage) VerifyRequestedServiceKeyboardOrder(ctx context.Context, ready bool) error {
	controls := []playwright.Locator{}
	if !ready {
		controls = append(controls, cp.page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Log in to GitHub Production", Exact: playwright.Bool(true)}))
	}
	for _, name := range []string{"GitHub Read Access", "GitHub Issue Triage"} {
		controls = append(controls, cp.permissionSetCard(name).GetByRole("switch", playwright.LocatorGetByRoleOptions{Name: "Toggle " + name, Exact: playwright.Bool(true)}))
	}
	if ready {
		controls = append(controls, cp.getConsentActionButton("Save"))
	}
	return cp.verifyKeyboardOrder(ctx, controls...)
}

func (sp *SessionsPage) VerifyServiceKeyboardOrder(ctx context.Context, active bool) error {
	if active {
		return sp.verifyKeyboardOrder(ctx, sp.refreshButtonLocator(), sp.terminateButtonLocator())
	}
	return sp.verifyKeyboardOrder(ctx, sp.loginButtonLocator())
}

func (sp *SessionsPage) VerifyTerminationCancellation(ctx context.Context) error {
	if err := sp.keyboardActivate(ctx, "Terminate"); err != nil {
		return err
	}
	if _, err := sp.page().WaitForFunction(`() => document.querySelector('[role="dialog"]')?.contains(document.activeElement)`, nil); err != nil {
		return fmt.Errorf("confirmation must receive keyboard focus: %w", err)
	}
	cancel := sp.page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Cancel session termination", Exact: playwright.Bool(true)})
	confirm := sp.page().GetByRole("dialog").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Terminate session with GitHub Production", Exact: playwright.Bool(true)})
	if err := sp.verifyKeyboardOrder(ctx, cancel, confirm); err != nil {
		return err
	}
	if err := confirm.Press("Tab"); err != nil {
		return err
	}
	inside, err := sp.page().GetByRole("dialog").Evaluate(`element => element.contains(document.activeElement)`, nil)
	if err != nil || inside != true {
		return fmt.Errorf("keyboard focus escaped confirmation: %v (%v)", inside, err)
	}
	if err := sp.page().Keyboard().Press("Escape"); err != nil {
		return err
	}
	_, err = sp.page().WaitForFunction(`() => document.activeElement?.textContent.trim() === 'Terminate' && !document.querySelector('[role="dialog"]')`, nil)
	return err
}

func (sp *SessionsPage) VerifyLoginFocus(_ context.Context) error {
	_, err := sp.page().WaitForFunction(`() => document.activeElement?.textContent.trim() === 'Login'`, nil)
	return err
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
