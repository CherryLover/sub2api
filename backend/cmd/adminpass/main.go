// Command adminpass 重置指定用户（通常是管理员）的登录密码。
//
// 邮件体系整体移除后，站点不再有"忘记密码"自助通道；本工具是被锁在门外时的
// 唯一兜底：拿到宿主机 shell 就能改密码，不依赖任何外部 SMTP 服务。
//
// 用法（容器内）：
//
//	docker exec -it <container> /app/adminpass -email admin@example.com
//	docker exec -i  <container> /app/adminpass -email admin@example.com -stdin <<<'new-password'
//	docker exec -e ADMINPASS_NEW_PASSWORD='new-password' <container> /app/adminpass -email admin@example.com
//
// 不传 -email 时改第一个管理员账号的密码。密码来源优先级：
// -password 参数 > ADMINPASS_NEW_PASSWORD 环境变量 > -stdin 从标准输入读一行。
// 生产环境建议用后两种，避免密码进入命令行历史与进程列表。
//
// 改写 password_hash 会让所有已签发的 access token 失效
// （token 版本由 email+password_hash 指纹推导），但存放在 Redis 里的
// refresh token 不在本工具的处理范围内，必要时请在面板里撤销全部会话。
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const minPasswordLength = 6

func main() {
	email := flag.String("email", "", "要改密码的账号邮箱（留空则取第一个管理员）")
	password := flag.String("password", "", "新密码（不安全：会进入命令行历史，建议用 ADMINPASS_NEW_PASSWORD 或 -stdin）")
	fromStdin := flag.Bool("stdin", false, "从标准输入读取一行作为新密码")
	flag.Parse()

	newPassword, err := resolveNewPassword(*password, *fromStdin)
	if err != nil {
		log.Fatalf("failed to resolve new password: %v", err)
	}

	cfg, err := config.LoadForBootstrap()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	client, sqlDB, err := repository.InitEnt(cfg)
	if err != nil {
		log.Fatalf("failed to init db: %v", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			log.Printf("failed to close db: %v", err)
		}
	}()

	userRepo := repository.NewUserRepository(client, sqlDB)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var user *service.User
	if strings.TrimSpace(*email) != "" {
		user, err = userRepo.GetByEmail(ctx, strings.TrimSpace(*email))
	} else {
		user, err = userRepo.GetFirstAdmin(ctx)
	}
	if err != nil {
		log.Fatalf("failed to resolve user: %v", err)
	}

	if err := user.SetPassword(newPassword); err != nil {
		log.Fatalf("failed to hash password: %v", err)
	}
	if err := userRepo.Update(ctx, user, service.UserUpdateFields{PasswordHash: true}); err != nil {
		log.Fatalf("failed to update password: %v", err)
	}

	fmt.Printf("password updated\nUSER_ID=%d\nEMAIL=%s\nROLE=%s\n", user.ID, user.Email, user.Role)
	fmt.Println("提示：已签发的 access token 立即失效；如需强制下线所有设备，请登录后在后台撤销全部会话。")
}

// resolveNewPassword 按 参数 > 环境变量 > 标准输入 的优先级取新密码并做长度校验。
func resolveNewPassword(flagValue string, fromStdin bool) (string, error) {
	if flagValue != "" {
		return validatePassword(flagValue)
	}
	if env := os.Getenv("ADMINPASS_NEW_PASSWORD"); env != "" {
		return validatePassword(env)
	}
	if fromStdin {
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("read password from stdin: %w", err)
		}
		return validatePassword(strings.TrimRight(line, "\r\n"))
	}
	return "", errors.New("no password provided: use -password, ADMINPASS_NEW_PASSWORD or -stdin")
}

func validatePassword(password string) (string, error) {
	if len(password) < minPasswordLength {
		return "", fmt.Errorf("password must be at least %d characters", minPasswordLength)
	}
	return password, nil
}
