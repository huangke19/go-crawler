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

// LoadSession 从文件加载 cookies
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

// SaveSession 保存 cookies 到文件
func SaveSession(cookies []*Cookie) error {
	data, err := json.MarshalIndent(cookies, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(sessionFile, data, 0600)
}

// Cookie 简化的 cookie 结构
type Cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	HTTPOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
}

// Login 执行登录流程
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

// SetCookies 设置 cookies 到浏览器
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

// EnsureLoggedIn 确保已登录，如果未登录则提示用户手动登录
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

// CreateBrowserContext 创建带有设备模拟的浏览器上下文
func CreateBrowserContext() (context.Context, context.CancelFunc) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false), // 登录时需要显示浏览器
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, _ := chromedp.NewContext(allocCtx)

	return ctx, cancel
}

// CreateFastBrowserContext 创建快速浏览器上下文（无头模式，禁用图片等）
func CreateFastBrowserContext() (context.Context, context.CancelFunc) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true), // 无头模式
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("disable-images", true),           // 禁用图片
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
