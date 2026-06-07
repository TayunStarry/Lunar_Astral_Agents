package proxy

import (
	"browser"
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

	"config"
	"logger"
)

// 存储生成的证书和私钥PEM数据
var (
	storedCertPEM []byte
	storedKeyPEM  []byte
)

// generateSelfSignedCert 生成或加载自签名TLS证书，优先从磁盘读取，不存在或已过期则重新生成并持久化
func generateSelfSignedCert() (tls.Certificate, error) {
	// 优先尝试从磁盘加载已有证书
	cert, err := loadCertFromDisk()
	if err == nil {
		logger.Info("ProxySvr", "从磁盘加载证书成功: %s", *config.CertFile)
		return cert, nil
	}
	logger.Info("ProxySvr", "磁盘证书不可用(%v)，将重新生成", err)

	// 生成新证书
	return generateAndSaveCert()
}

// loadCertFromDisk 尝试从磁盘加载证书，验证有效性
func loadCertFromDisk() (tls.Certificate, error) {
	certPEM, err := os.ReadFile(*config.CertFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("读取证书文件失败: %w", err)
	}
	keyPEM, err := os.ReadFile(*config.KeyFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("读取私钥文件失败: %w", err)
	}

	// 解析证书检查有效期
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return tls.Certificate{}, fmt.Errorf("证书PEM解析失败")
	}
	parsedCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("证书解析失败: %w", err)
	}

	// 检查证书是否过期（预留7天提前刷新）
	if time.Now().After(parsedCert.NotAfter.Add(-7 * 24 * time.Hour)) {
		return tls.Certificate{}, fmt.Errorf("证书即将过期或已过期，NotAfter=%s", parsedCert.NotAfter.Format("2006-01-02"))
	}

	// 存储以供ReadCertFile/ReadKeyFile使用
	storedCertPEM = certPEM
	storedKeyPEM = keyPEM

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("加载证书密钥对失败: %w", err)
	}
	return cert, nil
}

// generateAndSaveCert 生成新的自签名证书并保存到磁盘
func generateAndSaveCert() (tls.Certificate, error) {
	// 生成RSA私钥
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("生成RSA密钥失败: %v", err)
	}

	// 收集IP地址用于证书SAN
	var ipAddresses []net.IP
	ipAddresses = append(ipAddresses, net.ParseIP("127.0.0.1"))
	ipAddresses = append(ipAddresses, net.ParseIP("::1"))

	if ip, err := browser.GetLocalIP(nil); err == nil {
		if parsedIP := net.ParseIP(ip); parsedIP != nil {
			ipAddresses = append(ipAddresses, parsedIP)
		}
	}

	// 生成证书序列号
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("生成序列号失败: %v", err)
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
		return tls.Certificate{}, fmt.Errorf("创建证书失败: %v", err)
	}

	// 编码为PEM格式
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	// 保存到磁盘
	if err := saveCertToDisk(certPEM, keyPEM); err != nil {
		logger.Warn("ProxySvr", "证书持久化失败: %v，证书仅在内存中可用", err)
	} else {
		logger.Info("ProxySvr", "证书已持久化: %s", *config.CertFile)
	}

	// 存储以供ReadCertFile/ReadKeyFile使用
	storedCertPEM = certPEM
	storedKeyPEM = keyPEM

	// 加载TLS证书
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("加载证书失败: %v", err)
	}

	return cert, nil
}

// saveCertToDisk 将证书和私钥写入磁盘
func saveCertToDisk(certPEM, keyPEM []byte) error {
	certDir := filepath.Dir(*config.CertFile)
	if err := os.MkdirAll(certDir, 0755); err != nil {
		return fmt.Errorf("创建证书目录失败: %w", err)
	}

	if err := os.WriteFile(*config.CertFile, certPEM, 0644); err != nil {
		return fmt.Errorf("写入证书文件失败: %w", err)
	}

	keyDir := filepath.Dir(*config.KeyFile)
	if err := os.MkdirAll(keyDir, 0755); err != nil {
		return fmt.Errorf("创建私钥目录失败: %w", err)
	}

	if err := os.WriteFile(*config.KeyFile, keyPEM, 0600); err != nil {
		return fmt.Errorf("写入私钥文件失败: %w", err)
	}

	return nil
}
