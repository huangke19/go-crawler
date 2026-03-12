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
//   - SetCookies()：将 Cookie 注入到浏览器上下文
//   - EnsureLoggedIn()：验证登录状态，失败则返回错误
//   - CreateBrowserContext()：创建有头浏览器上下文（登录用）
//   - CreateFastBrowserContext()：创建无头浏览器上下文（爬取用）
//
// ============================================================================

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

const sessionFile = ".instagram_session.json"

// LoadSession 从 `.instagram_session.json` 加载浏览器 cookies。
// 该文件包含敏感登录态信息，通常应在 `.gitignore` 中忽略，并确保本地权限为仅用户可读写。
func LoadSession() ([]*Cookie, error) {
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return nil, err
	}

	var cookies []*Cookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return nil, err
	}

	return cookies, nil
}

// SaveSession 将 cookies 保存到 `.instagram_session.json`。
// 使用 0600 权限写入，尽量减少泄露风险（仅所有者可读写）。
func SaveSession(cookies []*Cookie) error {
	data, err := json.MarshalIndent(cookies, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(sessionFile, data, 0600)
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

// Login 执行“自动填写账号密码”的登录流程。
// 实际使用中通常推荐 `ManualLogin()`：可处理验证码/2FA 等需要人工交互的场景。
func Login(ctx context.Context, username, password string) error {
	fmt.Println("正在登录 Instagram...")

	err := chromedp.Run(ctx,
		chromedp.Navigate("https://www.instagram.com/accounts/login/"),

		// 等待登录表单加载
		chromedp.WaitVisible(`input[name="username"]`, chromedp.ByQuery),

		// 输入用户名和密码
		chromedp.SendKeys(`input[name="username"]`, username, chromedp.ByQuery),
		chromedp.SendKeys(`input[name="password"]`, password, chromedp.ByQuery),

		// 点击登录按钮
		chromedp.Click(`button[type="submit"]`, chromedp.ByQuery),

		// 等待登录完成（等待主页元素出现或登录按钮消失）
		chromedp.Sleep(3*time.Second),
	)

	if err != nil {
		return fmt.Errorf("登录失败: %v", err)
	}

	// 获取并保存 cookies
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
		return fmt.Errorf("获取 cookies 失败: %v", err)
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
// - 刷新页面后做一个轻量校验（是否仍出现“登录”入口）；
// - 若无法确认有效，则返回错误并提示先执行 `crawler login`。
//
// 该函数偏向“快速失败”：如果 cookie 过期，应尽快引导重新登录，避免后续 GraphQL/下载阶段才报错。
func EnsureLoggedIn(ctx context.Context) error {
	// 尝试加载已保存的 session
	cookies, err := LoadSession()
	if err == nil && len(cookies) > 0 {
		fmt.Println("找到已保存的 session，尝试使用...")

		// 先访问 Instagram 主页
		if err := chromedp.Run(ctx, chromedp.Navigate("https://www.instagram.com/")); err != nil {
			return err
		}

		// 设置 cookies
		if err := SetCookies(ctx, cookies); err != nil {
			fmt.Println("设置 cookies 失败，需要重新登录")
		} else {
			// 刷新页面验证登录状态
			if err := chromedp.Run(ctx,
				chromedp.Navigate("https://www.instagram.com/"),
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

// CreateBrowserContext 创建有头浏览器上下文（用于登录等需要人工交互的操作）。
// 这里会设置较“像真实用户”的 UA，并关闭部分自动化特征标记，以降低被页面脚本识别的概率。
func CreateBrowserContext() (context.Context, context.CancelFunc) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false), // 登录时需要显示浏览器
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, ctxCancel := chromedp.NewContext(allocCtx)

	// 返回组合的 cancel 函数
	cancel := func() {
		ctxCancel()
		allocCancel()
	}

	return ctx, cancel
}

// CreateFastBrowserContext 创建快速无头浏览器上下文（用于抓取/下载）。
// 关键点：
// - headless=true：减少资源占用；如需调试可改为 false；
// - 尝试禁用图片加载：降低带宽和页面渲染成本（但 GraphQL 媒体 URL 不依赖图片渲染）；\n+// - 添加 60 秒超时：避免 worker 长时间卡住；调用方应在任务完成后及时 cancel。
func CreateFastBrowserContext() (context.Context, context.CancelFunc) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true), // 无头模式
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("disable-images", true),                  // 禁用图片
		chromedp.Flag("blink-settings", "imagesEnabled=false"), // 禁用图片加载
		chromedp.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)

	// 添加 60 秒超时控制
	timeoutCtx, timeoutCancel := context.WithTimeout(allocCtx, 60*time.Second)

	ctx, ctxCancel := chromedp.NewContext(timeoutCtx)

	// 返回组合的 cancel 函数
	cancel := func() {
		ctxCancel()
		timeoutCancel()
		allocCancel()
	}

	return ctx, cancel
}
