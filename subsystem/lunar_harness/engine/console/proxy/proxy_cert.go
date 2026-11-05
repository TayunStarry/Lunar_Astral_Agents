package proxy

// proxy_cert.go 证书加载与生成

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

func loadOrGenerateCert(requiredIP string) (tls.Certificate, error) {
	certPath, keyPath := getCertPaths()
	// 优先尝试从磁盘加载
	cert, err := loadCertFromDisk(certPath, keyPath, requiredIP)
	if err == nil {
		logf("  从磁盘加载证书成功: %s", certPath)
		return cert, nil
	}
	logf("  磁盘证书不可用 (%v)，将重新生成", err)

	// 生成新证书
	return generateAndSaveCert(certPath, keyPath, requiredIP)
}

// loadCertFromDisk 从磁盘加载证书并验证有效性与 IP 覆盖
func loadCertFromDisk(certPath, keyPath, requiredIP string) (tls.Certificate, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("读取证书文件失败: %w", err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("读取私钥文件失败: %w", err)
	}

	// 解析证书检查有效期
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return tls.Certificate{}, fmt.Errorf("证书 PEM 解析失败")
	}
	parsedCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("证书解析失败: %w", err)
	}

	// 检查是否过期（预留 7 天提前刷新）
	if time.Now().After(parsedCert.NotAfter.Add(-7 * 24 * time.Hour)) {
		return tls.Certificate{}, fmt.Errorf("证书即将过期或已过期")
	}

	// 检查证书 SAN 是否覆盖所需 IP（回环与 localhost 除外）
	if !certCoversIP(parsedCert, requiredIP) {
		return tls.Certificate{}, fmt.Errorf("证书 SAN 未覆盖 IP %s", requiredIP)
	}

	return tls.X509KeyPair(certPEM, keyPEM)
}

// certCoversIP 检查证书 SAN 是否包含指定 IP（localhost/回环地址仅检查 DNS/回环覆盖）
func certCoversIP(cert *x509.Certificate, ipStr string) bool {
	if ipStr == "localhost" {
		return true
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return true
	}
	if ip.IsLoopback() {
		return true
	}
	for _, certIP := range cert.IPAddresses {
		if certIP.Equal(ip) {
			return true
		}
	}
	return false
}

// generateAndSaveCert 生成新的自签名证书并保存到磁盘，SAN 覆盖本机全部 IPv4 地址与所需 IP
func generateAndSaveCert(certPath, keyPath, requiredIP string) (tls.Certificate, error) {
	// 生成 RSA 私钥
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("生成 RSA 密钥失败: %w", err)
	}

	// 收集 IP 地址用于证书 SAN：回环 + 所有所选 IP + 本机全部可用 IPv4
	ipSet := make(map[string]net.IP)
	ipSet["127.0.0.1"] = net.ParseIP("127.0.0.1")
	ipSet["::1"] = net.ParseIP("::1")
	if parsed := net.ParseIP(requiredIP); parsed != nil {
		ipSet[requiredIP] = parsed
	}
	for _, candidate := range listLocalIPs() {
		if parsed := net.ParseIP(candidate.IP); parsed != nil {
			ipSet[candidate.IP] = parsed
		}
	}

	var ipAddresses []net.IP
	for _, ip := range ipSet {
		ipAddresses = append(ipAddresses, ip)
	}

	// 生成证书序列号
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("生成序列号失败: %w", err)
	}

	// 创建证书模板
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Lunar Astral Agents"},
			CommonName:   "localhost",
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: ipAddresses,
		DNSNames:    []string{"localhost"},
	}

	// 创建自签名证书
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("创建证书失败: %w", err)
	}

	// 编码为 PEM 格式
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	// 保存到磁盘
	if err := saveCertToDisk(certPath, keyPath, certPEM, keyPEM); err != nil {
		logf("  [WARN] 证书持久化失败: %v（证书仅在内存中可用）", err)
	} else {
		logf("  证书已持久化: %s", certPath)
	}

	return tls.X509KeyPair(certPEM, keyPEM)
}

// saveCertToDisk 将证书和私钥写入磁盘
func saveCertToDisk(certPath, keyPath string, certPEM, keyPEM []byte) error {
	certDirPath := filepath.Dir(certPath)
	if err := os.MkdirAll(certDirPath, 0755); err != nil {
		return fmt.Errorf("创建证书目录失败: %w", err)
	}

	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return fmt.Errorf("写入证书文件失败: %w", err)
	}

	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return fmt.Errorf("写入私钥文件失败: %w", err)
	}

	return nil
}

// ==== CORS 自动处理中间件 ====

// corsMiddleware 为所有响应添加 CORS 头，并处理 OPTIONS 预检请求
