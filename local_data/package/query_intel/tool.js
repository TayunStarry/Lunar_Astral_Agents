// query_intel LTP2 工具包 - 实时信息查询
// 由 goja 运行时加载执行，使用 syncFetch 进行网络请求
// 自动注册到 OnlyData.lunarToolPackageMap

(function () {
	/**
	 * 查询天气信息
	 * @param {string} sheng - 省份名称
	 * @param {string} place - 城市名称
	 * @returns {Promise<string>} 天气查询结果
	 */
	async function getWeather(sheng, place) {
		if (!sheng || !place) {
			return '天气查询需要提供省和市';
		}
		var url = 'https://cn.apihz.cn/api/tianqi/tqyb.php?id=88888888&key=88888888&sheng='
			+ encodeURIComponent(sheng) + '&place=' + encodeURIComponent(place);
		var result = syncFetch({ url: url, execute: { method: 'GET' } });
		var resp = result[0];
		var err = result[1];
		if (err) {
			return '天气查询失败: ' + err;
		}
		if (resp && resp.body) {
			return typeof resp.body === 'string' ? resp.body : JSON.stringify(resp.body);
		}
		return '天气数据为空';
	}

	/**
	 * 查询最新新闻
	 * @returns {Promise<string>} 新闻内容
	 */
	async function getNews() {
		var url = 'https://60s.7se.cn/v2/60s';
		var result = syncFetch({ url: url, execute: { method: 'GET' } });
		var resp = result[0];
		var err = result[1];
		if (err) {
			return '新闻查询失败: ' + err;
		}
		var newsData = resp && resp.body ? resp.body : null;
		if (newsData && newsData.data && newsData.data.news) {
			return newsData.data.news.join('\n');
		}
		return '未能获取新闻数据';
	}

	/**
	 * query_intel 工具主入口
	 * @param {Record<string, any> | string} args - 工具调用参数
	 * @returns {Promise<string>} 查询结果
	 */
	async function queryIntelHandler(args) {
		var params = typeof args === 'string' ? JSON.parse(args) : (args || {});

		switch (params.query_type) {
			case 'weather':
				return await getWeather(params.sheng, params.place);
			case 'news':
				return await getNews();
			default:
				return '不支持的查询类型: ' + params.query_type;
		}
	}

	// 注册到月华工具协议映射表
	// OnlyData 由 Go 适配器在执行此文件前注入为全局变量
	if (typeof OnlyData !== 'undefined' && OnlyData.lunarToolPackageMap) {
		OnlyData.lunarToolPackageMap.set('query_intel', queryIntelHandler);
		console.log('[LTP2] query_intel 工具注册成功');
	} else {
		console.error('[LTP2] OnlyData.lunarToolPackageMap 不可用，无法注册 query_intel');
	}
})();