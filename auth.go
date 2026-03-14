// ============================================================================
// auth.go - 认证管理与会话持久化
// ============================================================================
//
// 职责：
//   - 会话文件管理：加载/保存 Cookie 到 .instagram_session.json
//   - 浏览器上下文创建：有头模式（登录）/ 无头模式（爬取）
//   - 登录状态验证：确保 Cookie 有效，失败则提示重新登录
//   - Cookie 注入：将保存的 Cookie 恢复到浏览器上下文
//
// 核心概念：
//   - Cookie 是登录态的载体，保存在 .instagram_session.json（敏感文件，不提交 Git）
//   - 有头浏览器：用于手动登录，需要显示 UI（headless=false）
//   - 无头浏览器：用于自动爬取，不显示 UI（headless=true），禁用图片加载以提升速度
//
// 关键函数：
//   - LoadSession()：从文件加载 Cookie
//   - SaveSession()：保存 Cookie 到文件（权限 0600，仅所有者可读写）
//   - ExtractBrowserCookies()：从浏览器上下文中提取所有 Cookie
//   - SetCookies()：将 Cookie 注入到浏览器上下文
//   - EnsureLoggedIn()：验证登录状态，失败则返回错误
//   - ManualLogin()：打开有头浏览器，让用户手动完成登录
//   - CreateBrowserContext()：创建有头浏览器上下文（登录用）
//   - CreateFastBrowserContext()：创建无头浏览器上下文（爬取用）
//
// ============================================================================

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

const sessionFile = ".instagram_session.json"

// LoadSession 从 `.instagram_session.json` 加载浏览器 cookies。
// 该文件包含敏感登录态信息，通常应在 `.gitignore` 中忽略，并确保本地权限为仅用户可读写。
func LoadSession() ([]*Cookie, error) {
	var cookies []*Cookie
	if err := loadJSONFile(sessionFile, &cookies); err != nil {
		return nil, err
	}
	return cookies, nil
}

// SaveSession 将 cookies 保存到 `.instagram_session.json`。
// 使用 0600 权限写入，尽量减少泄露风险（仅所有者可读写）。
func SaveSession(cookies []*Cookie) error {
	return saveJSONFile(sessionFile, cookies, 0600)
}

// Cookie 是从浏览器侧导出的简化 cookie 结构（可 JSON 序列化）。
// 注意 `Expires` 来自浏览器协议，通常为 Unix 时间戳秒（浮点），用于恢复过期时间。
type Cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	HTTPOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
}

// ExtractBrowserCookies 从当前浏览器上下文中提取所有 Cookie。
func ExtractBrowserCookies(ctx context.Context) ([]*Cookie, error) {
	var cookies []*Cookie
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		cookiesData, err := network.GetCookies().Do(ctx)
		if err != nil {
			return err
		}
		for _, c := range cookiesData {
			cookies = append(cookies, &Cookie{
				Name:     c.Name,
				Value:    c.Value,
				Domain:   c.Domain,
				Path:     c.Path,
				Expires:  c.Expires,
				HTTPOnly: c.HTTPOnly,
				Secure:   c.Secure,
			})
		}
		return nil
	})); err != nil {
		return nil, fmt.Errorf("获取 cookies 失败: %v", err)
	}
	return cookies, nil
}

// Login 执行"自动填写账号密码"的登录流程。
// 实际使用中通常推荐 `ManualLogin()`：可处理验证码/2FA 等需要人工交互的场景。
func Login(ctx context.Context, username, password string) error {
	fmt.Println("正在登录 Instagram...")

	err := chromedp.Run(ctx,
		chromedp.Navigate(instagramLoginURL),

		// 等待登录表单加载
		chromedp.WaitVisible(`input[name="username"]`, chromedp.ByQuery),

		// 输入用户名和密码
		chromedp.SendKeys(`input[name="username"]`, username, chromedp.ByQuery),
		chromedp.SendKeys(`input[name="password"]`, password, chromedp.ByQuery),

		// 点击登录按钮
		chromedp.Click(`button[type="submit"]`, chromedp.ByQuery),

		// 等待登录完成（等待主页元素出现或登录按钮消失）
		chromedp.Sleep(loginActionWait),
	)

	if err != nil {
		return fmt.Errorf("登录失败: %v", err)
	}

	// 获取并保存 cookies
	cookies, err := ExtractBrowserCookies(ctx)
	if err != nil {
		return err
	}

	if err := SaveSession(cookies); err != nil {
		return fmt.Errorf("保存 session 失败: %v", err)
	}

	fmt.Println("登录成功！Session 已保存")
	return nil
}

// SetCookies 将 cookies 注入到浏览器上下文中。
// 注：这里的 cookie 只负责恢复登录态；是否有效仍需要后续通过页面行为或接口响应来验证。
func SetCookies(ctx context.Context, cookies []*Cookie) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		for _, c := range cookies {
			expr := cdp.TimeSinceEpoch(time.Unix(int64(c.Expires), 0))
			if err := network.SetCookie(c.Name, c.Value).
				WithDomain(c.Domain).
				WithPath(c.Path).
				WithHTTPOnly(c.HTTPOnly).
				WithSecure(c.Secure).
				WithExpires(&expr).
				Do(ctx); err != nil {
				return err
			}
		}
		return nil
	}))
}

// EnsureLoggedIn 确保当前浏览器上下文具备有效登录态。
// 流程：
// - 尝试从本地 session 文件加载 cookies 并注入浏览器；
// - 刷新页面后做一个轻量校验（是否仍出现"登录"入口）；
// - 若无法确认有效，则返回错误并提示先执行 `crawler login`。
//
// 该函数偏向"快速失败"：如果 cookie 过期，应尽快引导重新登录，避免后续 GraphQL/下载阶段才报错。
func EnsureLoggedIn(ctx context.Context) error {
	// 尝试加载已保存的 session
	cookies, err := LoadSession()
	if err == nil && len(cookies) > 0 {
		fmt.Println("找到已保存的 session，尝试使用...")

		// 先访问 Instagram 主页
		if err := chromedp.Run(ctx, chromedp.Navigate(instagramBaseURL+"/")); err != nil {
			return err
		}

		// 设置 cookies
		if err := SetCookies(ctx, cookies); err != nil {
			fmt.Println("设置 cookies 失败，需要重新登录")
		} else {
			// 刷新页面验证登录状态
			if err := chromedp.Run(ctx,
				chromedp.Navigate(instagramBaseURL+"/"),
				chromedp.WaitReady("body"),
			); err != nil {
				return err
			}

			// 检查是否登录成功（简单检查：看是否有登录按钮）
			var loginBtnExists bool
			chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('a[href="/accounts/login/"]') !== null`, &loginBtnExists))

			if !loginBtnExists {
				fmt.Println("✓ Session 有效，已登录")
				return nil
			}
			fmt.Println("Session 已过期，需要重新登录")
		}
	}

	// 需要重新登录
	return fmt.Errorf("未找到有效的登录 session，请先运行: go run . login")
}

// createBrowserContext 创建浏览器上下文的内部实现。
// headless 控制是否无头模式，timeout 控制整体超时（0 表示不设超时）。
// 公共配置：伪装 UA + 关闭自动化特征标记，降低被 Instagram 脚本识别的概率。
func createBrowserContext(headless bool, timeout time.Duration) (context.Context, context.CancelFunc) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	if headless {
		opts = append(opts,
			chromedp.Flag("headless", true),
			chromedp.Flag("disable-images", true),
			chromedp.Flag("blink-settings", "imagesEnabled=false"),
		)
	} else {
		opts = append(opts,
			chromedp.Flag("headless", false),
		)
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)

	if timeout > 0 {
		timeoutCtx, timeoutCancel := context.WithTimeout(allocCtx, timeout)
		ctx, ctxCancel := chromedp.NewContext(timeoutCtx)
		cancel := func() {
			ctxCancel()
			timeoutCancel()
			allocCancel()
		}
		return ctx, cancel
	}

	ctx, ctxCancel := chromedp.NewContext(allocCtx)
	cancel := func() {
		ctxCancel()
		allocCancel()
	}
	return ctx, cancel
}

// CreateBrowserContext 创建有头浏览器上下文（用于登录等需要人工交互的操作）。
func CreateBrowserContext() (context.Context, context.CancelFunc) {
	return createBrowserContext(false, 0)
}

// CreateFastBrowserContext 创建快速无头浏览器上下文（用于抓取/下载）。
// headless + 禁用图片 + 60 秒超时，调用方应在任务完成后及时 cancel。
func CreateFastBrowserContext() (context.Context, context.CancelFunc) {
	return createBrowserContext(true, browserFastTimeout)
}

// ManualLogin 手动登录流程：打开有头浏览器，让用户完成登录（支持验证码/2FA）。
func ManualLogin() error {
	fmt.Println("=== Instagram 手动登录 ===")
	fmt.Println("即将打开浏览器，请手动完成登录...")

	// 创建浏览器上下文
	ctx, cancel := CreateBrowserContext()
	defer cancel()

	// 打开 Instagram 登录页
	err := chromedp.Run(ctx,
		chromedp.Navigate(instagramLoginURL),
	)

	if err != nil {
		return fmt.Errorf("打开登录页面失败: %v", err)
	}

	// 等待用户手动登录
	fmt.Println("\n请在浏览器中完成登录（包括验证码、双因素认证等）")
	fmt.Println("登录完成后，回到这里按回车键继续...")
	fmt.Scanln()

	// 保存 cookies。这里直接从浏览器读取当前 cookie jar，
	// 以便后续在无头上下文中复用登录态调用 GraphQL / 下载资源。
	fmt.Println("正在保存登录状态...")
	cookies, err := ExtractBrowserCookies(ctx)
	if err != nil {
		return err
	}

	if err := SaveSession(cookies); err != nil {
		return fmt.Errorf("保存 session 失败: %v", err)
	}

	fmt.Println("✓ 登录状态已保存！")
	fmt.Println("浏览器将在 2 秒后关闭...")
	time.Sleep(pageLoadWait)

	return nil
}
