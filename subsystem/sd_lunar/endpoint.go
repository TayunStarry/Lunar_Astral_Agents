package main

// SystemEndpoints 系统端点列表
var SystemEndpoints = []SystemEndpoint{
	{Path: "/sd/txt2img", Handler: txt2imgHandler, Method: "POST", Description: "文生图生成"},
	{Path: "/sd/img2img", Handler: img2imgHandler, Method: "POST", Description: "图生图生成"},
	{Path: "/sd/status", Handler: statusHandler, Method: "GET", Description: "任务状态查询"},
	{Path: "/sd/poll/", Handler: pollHandler, Method: "GET", Description: "轮询任务完成"},
	{Path: "/sd/result/", Handler: resultHandler, Method: "GET", Description: "获取生成结果"},
	{Path: "/sd/config", Handler: configHandler, Method: "GET", Description: "系统配置信息"},
}
