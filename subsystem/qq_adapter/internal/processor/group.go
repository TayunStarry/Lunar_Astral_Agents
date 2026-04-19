package processor

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"subsystem/internal/setup"
	"subsystem/internal/utils"
)

// ValidateListenGroups 校验需要监听的群
func (class *Handle) ValidateListenGroups() []int64 {
	validGroupIDs := make([]int64, 0)

	// 如果没有配置需要监听的群，返回空切片
	if len(class.Config.ListenGroupIDs) == 0 {
		log.Println("没有配置需要监听的群")
		return validGroupIDs
	}

	// 遍历需要监听的群ID
	for _, groupID := range class.Config.ListenGroupIDs {
		// 检查群是否在群列表中
		found := false
		for _, group := range class.groupInfos {
			if group.GroupID == groupID {
				found = true
				break
			}
		}

		if found {
			validGroupIDs = append(validGroupIDs, groupID)
			log.Printf("群 ID %d 校验通过", groupID)
		} else {
			log.Printf("群 ID %d 不在群列表中，跳过", groupID)
		}
	}

	return validGroupIDs
}

// ParseGroupListResponse 解析群列表响应
func (class *Handle) ParseGroupListResponse(message []byte) error {
	var response utils.WSResponse
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
			class.groupInfos = make([]setup.GroupInfo, 0, len(groupList))
			for _, item := range groupList {
				if group, ok := item.(map[string]interface{}); ok {
					groupID := int64(utils.GetFloat64Value(group, "group_id"))
					groupName := utils.GetStringValue(group, "group_name")
					memberCount := int(utils.GetFloat64Value(group, "member_count"))
					maxMemberCount := int(utils.GetFloat64Value(group, "max_member_count"))

					if groupID > 0 {
						class.groupInfos = append(class.groupInfos, setup.GroupInfo{
							GroupID:        groupID,
							GroupName:      groupName,
							MemberCount:    memberCount,
							MaxMemberCount: maxMemberCount,
						})
					}
				}
			}

			// 打印群列表
			log.Printf("可选的群聊数量 -> %d", len(class.groupInfos))
			for _, group := range class.groupInfos {
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
func (class *Handle) ParseGroupMemberListResponse(message []byte) error {
	var response utils.WSResponse
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
				if _, ok := class.groupMembers[groupID]; !ok {
					class.groupMembers[groupID] = make(map[int64]string)
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
							class.groupMembers[groupID][userID] = nickname
						}
					}
				}

				log.Printf("正在订阅 -> QQ群 %d", groupID)
			}
		}
	}

	return nil
}
