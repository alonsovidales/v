var Metric = (function() {
	var my = {
		plot: function(targetDiv, currency) {
			$.getJSON(
				'http://bigbrain:8888/get_curr_values_orders?curr=' + currency,
				function(info) {
					var buyFlag = [];
					var sellFlag = [];
					var usedTs = {};
					$.each(info.Orders, function(i, v) {
						if (!usedTs[v.SellTs]) {
							usedTs[v.SellTs] = true;
							sellFlag.push({
								x: Math.round(v.SellTs/1000000),
								title: 'S:' + i,
								text: 'Profit: ' + v.Profit
							});
							buyFlag.push({
								x: Math.round(v.BuyTs/1000000),
								title: 'B:' + i,
								text: 'Shape: "squarepin"'
							});
						}
					});
					var askPrices = [];
					var bidPrices = [];
					$.each(info.Prices, function(_, v) {
						var ts = Math.round(v.t/1000000);
						askPrices.push([ts, v.a]);
						bidPrices.push([ts, v.b]);
					});

					$('#' + targetDiv).highcharts({
						chart: {
							type: 'spline',
							zoomType: 'xy'
						},
						title: {
							text: currency
						},
						xAxis: {
							type: 'datetime',
						},
						tooltip: {
							shared: true
						},

						plotOptions: {
							spline: {
								lineWidth: 1,
								marker: {
									enabled: false
								}
							}
						},

						series: [{
							name: "Ask",
							id: "ask",
							data: askPrices
						}, {
							name: "Bid",
							id: "bid",
							data: bidPrices
						}, {
							type: 'flags',
							data: buyFlag,
							onSeries: 'ask',
							shape: 'squarepin',
							width: 16
						}, {
							type: 'flags',
							data: sellFlag,
							onSeries: 'bid',
							shape: 'squarepin',
							width: 16
						}]
					});
				}
			);
		}
	};

	return my;
})();
