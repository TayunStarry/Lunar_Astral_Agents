package MutualForum

// naming.go 首次申领时的网名生成。
//
// 月华在登录论坛时给自己起一个网名：默认由本地词池组合生成并落盘，
// 之后一直复用；配置里显式写了非默认名称时以人工配置为准。

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

// 词池：前半是气质词，后半是意象词，组合成「拾光·青岚」这类 2+2 网名。
var namePrefixes = []string{
	"拾光", "晚风", "星屑", "白露", "澄澈", "微光", "静水", "轻霜",
	"初雪", "远山", "浮云", "南栀", "北辰", "薄暮", "晴岚", "疏影",
}

var nameSuffixes = []string{
	"月华", "青岚", "流萤", "奶糖", "霜降", "渡鸦", "白鹭", "松间",
	"知夏", "照影", "半盏", "听澜", "归舟", "饮冰", "眠云", "拾贝",
}

// GenerateName 组合一个网名；失败时返回错误，由调用方回退到默认名。
func GenerateName() (string, error) {
	prefix, err := pickName(namePrefixes)
	if err != nil {
		return "", err
	}
	suffix, err := pickName(nameSuffixes)
	if err != nil {
		return "", err
	}
	return prefix + "·" + suffix, nil
}

// chooseName 决定申领时提交的名称。
//
//   - auto_name=false：完全使用配置里的 instance_name；
//   - 配置里写了「月华」以外的名称：视为人工指定，直接使用；
//   - 其余情况：由月华自己生成网名。
func chooseName(cfg Config) (name string, generated bool, err error) {
	explicit := strings.TrimSpace(cfg.InstanceName)
	if !cfg.AutoName {
		if explicit == "" {
			explicit = "月华"
		}
		return explicit, false, nil
	}
	if explicit != "" && explicit != "月华" {
		return explicit, false, nil
	}

	generatedName, genErr := GenerateName()
	if genErr != nil {
		return "月华", false, nil
	}
	return generatedName, true, nil
}

func pickName(pool []string) (string, error) {
	if len(pool) == 0 {
		return "", fmt.Errorf("词池为空")
	}
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(pool))))
	if err != nil {
		return "", err
	}
	return pool[index.Int64()], nil
}
