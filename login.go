package main

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// ManualLogin 手动登录流程
func ManualLogin() error {
	fmt.Println("=== Instagram 手动登录 ===")
	fmt.Println("即将打开浏览器，请手动完成登录...")

	// 创建浏览器上下文
	ctx, cancel := CreateBrowserContext()
	defer cancel()

	// 打开 Instagram 登录页
	err := chromedp.Run(ctx,
		chromedp.Navigate("https://www.instagram.com/accounts/login/"),
	)

	if err != nil {
		return fmt.Errorf("打开登录页面失败: %v", err)
	}

	// 等待用户手动登录
	fmt.Println("\n请在浏览器中完成登录（包括验证码、双因素认证等）")
	fmt.Println("登录完成后，回到这里按回车键继续...")
	fmt.Scanln()

	// 保存 cookies
	fmt.Println("正在保存登录状态...")
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

	fmt.Println("✓ 登录状态已保存！")
	fmt.Println("浏览器将在 2 秒后关闭...")
	time.Sleep(2 * time.Second)

	return nil
}
