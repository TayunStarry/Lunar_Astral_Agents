package message

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"nap_cat_bridging/internal/models"
	"nap_cat_bridging/internal/utils"
	"nap_cat_bridging/pkg/websocket"
)

// Parser 消息解析器
type Parser struct {
	handler *Handler
}

// NewParser 创建消息解析器
func NewParser(handler *Handler) *Parser {
	return &Parser{
		handler: handler,
	}
}

// ParseGroupListResponse 解析群列表响应
func (p *Parser) ParseGroupListResponse(message []byte) error {
	var response websocket.WSResponse
	if err := json.Unmarshal(message, &response); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}

	// 检查是否是get_group_list的响应
	if !strings.Contains(response.Echo, "get_group_list_") {
		return fmt.Errorf("不是get_group_list的响应")
	}

	// 解析群列表数据
	if response.Status == "ok" && response.Data != nil {
		if groupList, ok := response.Data.([]interface{}); ok {
			p.handler.groupInfos = make([]models.GroupInfo, 0, len(groupList))
			for _, item := range groupList {
				if group, ok := item.(map[string]interface{}); ok {
					groupID := int64(utils.GetFloat64Value(group, "group_id"))
					groupName := utils.GetStringValue(group, "group_name")
					memberCount := int(utils.GetFloat64Value(group, "member_count"))
					maxMemberCount := int(utils.GetFloat64Value(group, "max_member_count"))

					if groupID > 0 {
						p.handler.groupInfos = append(p.handler.groupInfos, models.GroupInfo{
							GroupID:        groupID,
							GroupName:      groupName,
							MemberCount:    memberCount,
							MaxMemberCount: maxMemberCount,
						})
					}
				}
			}

			// 打印群列表
			log.Printf("可选的群聊数量 -> %d", len(p.handler.groupInfos))
			for _, group := range p.handler.groupInfos {
				log.Printf("%s", strings.Repeat("-", 32))
				log.Printf("群 ID: %d", group.GroupID)
				log.Printf("成员数: %d/%d", group.MemberCount, group.MaxMemberCount)
				log.Printf("群名称: < %s >", group.GroupName)
			}
		}
	}

	return nil
}

// ParseGroupMemberListResponse 解析群成员列表响应
func (p *Parser) ParseGroupMemberListResponse(message []byte) error {
	var response websocket.WSResponse
	if err := json.Unmarshal(message, &response); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}

	// 检查是否是get_group_member_list的响应
	if !strings.Contains(response.Echo, "get_group_member_list_") {
		return fmt.Errorf("不是get_group_member_list的响应")
	}

	// 解析群成员列表数据
	if response.Status == "ok" && response.Data != nil {
		if memberList, ok := response.Data.([]interface{}); ok {
			// 提取群ID
			var groupID int64
			for _, item := range memberList {
				if member, ok := item.(map[string]interface{}); ok {
					if gid, ok := member["group_id"].(float64); ok {
						groupID = int64(gid)
						break
					}
				}
			}

			if groupID > 0 {
				// 初始化该群的成员映射
				if _, ok := p.handler.groupMembers[groupID]; !ok {
					p.handler.groupMembers[groupID] = make(map[int64]string)
				}

				// 填充成员信息
				for _, item := range memberList {
					if member, ok := item.(map[string]interface{}); ok {
						userID := int64(utils.GetFloat64Value(member, "user_id"))
						nickname := ""

						// 优先使用群名片，然后使用昵称
						if card, ok := member["card"].(string); ok && card != "" {
							nickname = card
						} else if nick, ok := member["nickname"].(string); ok {
							nickname = nick
						}

						if userID > 0 && nickname != "" {
							p.handler.groupMembers[groupID][userID] = nickname
						}
					}
				}

				log.Printf("正在订阅 -> QQ群 %d", groupID)
			}
		}
	}

	return nil
}

// ParseMessageResponse 解析消息响应
func (p *Parser) ParseMessageResponse(data map[string]any) (map[string]any, error) {
	// 提取发送者信息
	senderName := p.handler.getSenderName(data)

	// 处理消息内容
	content := p.handler.processor.ProcessOriginalMessageContent(data)

	// 返回结果
	return map[string]any{
		"sender":  senderName,
		"content": content,
	}, nil
}
