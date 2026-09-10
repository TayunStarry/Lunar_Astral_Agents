// 沉默插件 - com.yaraflow.silence

var DEFAULT_CONFIG_YAML = [
  "# 沉默插件 - 配置文件",
  "# 此文件由插件首次运行时自动生成",
  "",
  "# 白名单认证：控制插件在哪些群聊/私聊中可用",
  "# 启用后，只有白名单中的群聊和私聊可以使用本插件的工具和命令",
  "auth:",
  "  enabled: false",
  "  allowed_groups: []",
  '  # 示例: ["qq:160063908", "qq:123456789"]',
  "  allowed_private: []",
  '  # 示例: ["qq:123456789"]',
  "",
  "components:",
  "  enable_silence_tool: true",
  "  enable_silence_command: true",
  "  enable_silence_hook: true",
  "",
  "permissions:",
  "  white_or_black_list: whitelist",
  "  admin_users:",
  '    - "qq:你的QQ号"',
  "",
  "adjustment:",
  "  low_case_min: 120",
  "  low_case_max: 600",
  "  medium_case_min: 600",
  "  medium_case_max: 1200",
  "  serious_case_min: 1200",
  "  serious_case_max: 5400",
  "  max_action_silence_time: 10800",
  "",
  "experimental:",
  "  silence_special_check: false",
  '  silence_someone_list: []',
  '  silence_group_list: []',
  ""
].join("\n");

var _silenceRecords = {};

function ensureConfigFile() {
  try {
    var config = yara.config.getFile();
    if (config && Object.keys(config).length > 0) return config;
  } catch (e) {}

  yara.logger.info("沉默插件: 配置文件不存在，自动生成默认配置");
  try {
    yara.file.write("config.yaml", DEFAULT_CONFIG_YAML);
    yara.logger.info("沉默插件: 默认配置文件已生成");
  } catch (e) {
    yara.logger.error("沉默插件: 无法生成配置文件: " + e.message);
  }

  try {
    return yara.config.getFile();
  } catch (e) {
    yara.logger.error("沉默插件: 无法读取配置文件: " + e.message);
    return {};
  }
}

function getConfig() {
  return ensureConfigFile();
}

function getConfigValue(path, defaultValue) {
  var config = getConfig();
  var keys = path.split(".");
  var val = config;
  for (var i = 0; i < keys.length; i++) {
    if (val === null || val === undefined || typeof val !== "object") return defaultValue;
    val = val[keys[i]];
  }
  return (val !== undefined && val !== null) ? val : defaultValue;
}

function checkUserPermission(userId, platform) {
  var mode = getConfigValue("permissions.white_or_black_list", "whitelist");
  var adminUsers = getConfigValue("permissions.admin_users", []);
  if (!adminUsers || adminUsers.length === 0) return false;

  var userKey = platform + ":" + userId;
  var inList = false;
  for (var i = 0; i < adminUsers.length; i++) {
    if (adminUsers[i] === userKey) { inList = true; break; }
  }

  return mode === "whitelist" ? inList : !inList;
}

function addSilence(groupId, caseType, duration) {
  if (caseType === "low") {
    var lowMin = getConfigValue("adjustment.low_case_min", 120);
    var lowMax = getConfigValue("adjustment.low_case_max", 600);
    duration = Math.floor(Math.random() * (lowMax - lowMin + 1)) + lowMin;
  } else if (caseType === "medium") {
    var medMin = getConfigValue("adjustment.medium_case_min", 600);
    var medMax = getConfigValue("adjustment.medium_case_max", 1200);
    duration = Math.floor(Math.random() * (medMax - medMin + 1)) + medMin;
  } else if (caseType === "serious") {
    if (duration === null || duration === undefined) {
      var serMin = getConfigValue("adjustment.serious_case_min", 1200);
      var serMax = getConfigValue("adjustment.serious_case_max", 5400);
      duration = Math.floor(Math.random() * (serMax - serMin + 1)) + serMin;
    } else {
      var maxAction = getConfigValue("adjustment.max_action_silence_time", 10800);
      if (duration > maxAction) {
        yara.logger.warn("沉默插件: 请求沉默时间(" + duration + "秒)超过最大限制(" + maxAction + "秒)");
        return false;
      }
    }
  } else if (caseType === "command") {
    // duration 保持调用者传入的值
  } else {
    yara.logger.error("沉默插件: 无效的沉默类型: " + caseType);
    return false;
  }

  var expiration = null;
  if (duration !== null && duration !== undefined && duration > 0) {
    expiration = yara.time.now() + duration;
  }

  _silenceRecords[groupId] = { expiration: expiration, type: caseType };
  var durStr = (expiration !== null) ? (duration + "秒") : "永久";
  yara.logger.info("沉默插件: 群组 " + groupId + " 进入沉默 [" + caseType + "] " + durStr);
  return true;
}

function removeSilence(groupId) {
  if (!(groupId in _silenceRecords)) return false;
  delete _silenceRecords[groupId];
  yara.logger.info("沉默插件: 群组 " + groupId + " 已解除沉默");
  return true;
}

function isSilenced(groupId) {
  if (!groupId || !(groupId in _silenceRecords)) return { silenced: false, type: "" };

  var record = _silenceRecords[groupId];
  if (record.expiration === null || record.expiration === undefined) {
    return { silenced: true, type: "force_silence" };
  }

  if (yara.time.now() >= record.expiration) {
    delete _silenceRecords[groupId];
    yara.logger.info("沉默插件: 群组 " + groupId + " 沉默已过期，自动清理");
    return { silenced: false, type: "" };
  }

  return { silenced: true, type: "" };
}

function isSilencedSpecial(userId, groupId, platform) {
  if (!getConfigValue("experimental.silence_special_check", false)) return false;

  var someoneList = getConfigValue("experimental.silence_someone_list", []);
  var groupList = getConfigValue("experimental.silence_group_list", []);

  if (userId && someoneList && someoneList.length > 0) {
    var userKey = platform + ":" + userId;
    for (var i = 0; i < someoneList.length; i++) {
      if (someoneList[i] === userKey) return true;
    }
  }

  if (groupId && groupList && groupList.length > 0) {
    var groupKey = platform + ":" + groupId;
    for (var i = 0; i < groupList.length; i++) {
      if (groupList[i] === groupKey) return true;
    }
  }

  return false;
}

if (getConfigValue("components.enable_silence_hook", true)) {
  yara.hook.register("chat.receive.before_process", function(event) {
    var msg = event.message;
    if (!msg) return { allowContinue: true };

    var groupId = msg.groupId;
    var userId = msg.senderId;
    var platform = msg.platform || "qq";

    var silenceResult = isSilenced(groupId);

    if (!silenceResult.silenced && isSilencedSpecial(userId, groupId, platform)) {
      addSilence(groupId, "command", null);
      silenceResult = { silenced: true, type: "special_silence" };
    }

    if (silenceResult.silenced) {
      if (msg.isAtMe && silenceResult.type !== "force_silence") {
        yara.logger.info("沉默插件: 群组 " + groupId + " 被@，解除沉默");
        removeSilence(groupId);
        return { allowContinue: true };
      }

      if (silenceResult.type === "force_silence" && msg.isAtMe) {
        yara.logger.info("沉默插件: 群组 " + groupId + " 强制沉默中，@无法打断");
      }

      return { allowContinue: true, logSuffix: "(窥屏ing)" };
    }

    return { allowContinue: true };
  }, { mode: "blocking", order: "early" });

  yara.hook.register("chat.receive.after_process", function(event) {
    var msg = event.message;
    if (!msg) return { allowContinue: true };

    var groupId = msg.groupId;

    if (isSilenced(groupId).silenced) {
      return { allowContinue: false };
    }

    return { allowContinue: true };
  }, { mode: "blocking", order: "early" });
}

if (getConfigValue("components.enable_silence_tool", true)) {
  yara.tool.register("set_silence", {
    description: "让语瞳进入沉默状态，暂停回复消息。当以下情况调用：1) 你觉得自己话太多或有人反映你话太多；2) 有人说的话不合时宜、不够专业、具备误导性，聊天气氛不对；3) 有人明确且礼貌地要求你保持沉默。注意：如果有人蛮横无理地要求你闭嘴并带有侮辱性质，不要使用。",
    parameters: [
      { name: "case", type: "string", description: "情况级别。low=适当收敛，medium=说错话/气氛不对，serious=被人明确要求", required: true },
      { name: "time", type: "integer", description: "沉默时长（秒），选填。仅在被人明确要求沉默多久时填入", required: false }
    ],
    toolType: "agent",
    visibility: "visible"
  }, function(params, context) {
    var caseType = params.case;
    var duration = params.time;
    var groupId = (context && context.groupId) ? context.groupId : "";

    if (!groupId) return "错误: 无法获取当前群组ID";

    if (isSilenced(groupId).silenced) return "群组 " + groupId + " 已经处于沉默状态";

    var validCases = ["low", "medium", "serious"];
    var isValid = false;
    for (var i = 0; i < validCases.length; i++) {
      if (caseType === validCases[i]) { isValid = true; break; }
    }
    if (!isValid) return "错误: 无效的情况级别 '" + caseType + "'，请使用 low、medium 或 serious";

    if (addSilence(groupId, caseType, duration)) {
      return "已进入沉默状态 [" + caseType + "]，静默期结束后自动恢复。";
    }
    return "沉默设置失败，可能沉默时间超过最大限制。";
  });
}

if (getConfigValue("components.enable_silence_command", true)) {
  yara.command.register("silence", "^/silence\\s+(?P<action>true|false)(?:\\s+(?P<duration>\\d+))?\\s*$", function(match, context) {
    var action = match.action;
    var duration = match.duration;
    var groupId = (context && context.groupId) ? context.groupId : "";
    var userId = (context && context.userId) ? context.userId : "";
    var platform = (context && context.platform) ? context.platform : "qq";

    if (!groupId) return "错误: 该命令只能在群聊中使用";

    if (!checkUserPermission(userId, platform)) return "权限不足，你无权使用沉默命令";

    if (action === "true") {
      if (isSilenced(groupId).silenced) removeSilence(groupId);

      var durationVal = (duration !== undefined && duration !== null) ? parseInt(duration, 10) : null;
      if (addSilence(groupId, "command", durationVal)) {
        var durStr = (durationVal !== null) ? (durationVal + "秒") : "永久";
        return "已对群组 " + groupId + " 设置沉默，持续时间: " + durStr;
      }
      return "设置沉默失败";
    }

    if (action === "false") {
      if (removeSilence(groupId)) return "已解除群组 " + groupId + " 的沉默状态";
      return "群组 " + groupId + " 未处于沉默状态";
    }

    return "未知操作";
  });
}

function onLoad() {
  yara.logger.info("沉默插件已加载");
  ensureConfigFile();
}

function onUnload() {
  yara.logger.info("沉默插件已卸载");
  _silenceRecords = {};
}