'use strict';

angular.module('sidecar.services', ['ngRoute', 'ui.bootstrap'])

.config(['$routeProvider', function($routeProvider) {
  $routeProvider.when('/services', {
    templateUrl: 'services/services.html',
    controller: 'servicesCtrl'
  });
}])

.factory('stateService', function($http) {
	var state = {};

	state.getServices = function() {
      return $http({
        method: 'GET', 
        url: '/services.json',
		dataType: 'json',
      });
    }

    return state;
})

.controller('servicesCtrl', function($scope, $interval, stateService) {
    $scope.serverList = {};
	$scope.clusterName = "";
	$scope.servicesList = {};
	$scope.collapsed = {};

	$scope.toggleCollapse = function(svcName) {
		$scope.collapsed[svcName] = !$scope.isCollapsed(svcName);
	};

	$scope.isCollapsed = function(svcName) {
		return $scope.collapsed[svcName] == null || $scope.collapsed[svcName];
	};
	
	var getData = function() {
    	stateService.getServices().success(function (response) {
			var services = {};
			for (var svcName in response.Services) {
				services[svcName] = response.Services[svcName].groupBy(function(s) {
					var ports = _.map(s.Ports, function(p) { _.pick(p, 'ServicePort') });
					return [s.Image, ports, s.Status];
				});
				if ($scope.collapsed[svcName] == null) {
					$scope.collapsed[svcName] = true;
				}
			}
			$scope.servicesList = services;
			$scope.clusterName = response.ClusterName;
			$scope.serverList = response.ClusterMembers;
    	});
	};

	getData();
	$interval(getData, 4000); // every 4 seconds
})

.filter('portsStr', function() {
	return function(svcPorts) {
		var ports = []
		for (var i in svcPorts) {
			var port = svcPorts[i]

			if (port.Port == null) {
				continue;
			}

			if (port.ServicePort != null && port.ServicePort != 0) {
				ports.push(port.ServicePort.toString() + "->" + port.Port.toString())
			} else {
				ports.push(port.Port.toString())
			}
		}
	
		return ports.join(", ")
	};
})

.filter('statusStr', function() {
	return function(status) {
	    switch (status) {
	    case 0:
	        return "Alive"
	    case 2:
	        return "Unhealthy"
	    case 3:
	        return "Unknown"
	    default:
	        return "Tombstone"
	    }
	}
})

.filter('timeAgo', function() {
	return function(textDate) {
		if (textDate == null || textDate == "" || textDate == "1970-01-01T01:00:00+01:00") {
			return "never";
		}

		var date = Date.parse(textDate)
	    var seconds = Math.floor((new Date() - date) / 1000);
	
	    var interval = Math.floor(seconds / 31536000);
	
	    if (interval > 1) {
	        return interval + " years ago";
	    }
	    interval = Math.floor(seconds / 2592000);
	    if (interval > 1) {
	        return interval + " months ago";
	    }
	    interval = Math.floor(seconds / 86400);
	    if (interval > 1) {
	        return interval + " days ago";
	    }
	    interval = Math.floor(seconds / 3600);
	    if (interval > 1) {
	        return interval + " hours ago";
	    }
	    interval = Math.floor(seconds / 60);
	    if (interval > 1) {
	        return interval + " mins ago";
	    }
	    return Math.floor(seconds) + " secs ago";
	}
})

.filter('imageStr', function() {
	return function(imageName) {
		if (imageName.length < 25) {
			return imageName;
		}

		return imageName.substr(imageName.length - 25, imageName.length)
	}
})

.filter('extractTag', function() {
	return function(imageName) {
		return imageName.split(':')[1]
	}
})

;

if ( ! Array.prototype.groupBy) {
  Array.prototype.groupBy = function (f)
  {
    var groups = {};
    this.forEach(function(o) {
      var group = JSON.stringify(f(o));
      groups[group] = groups[group] || [];
      groups[group].push(o);  
    });
    
    return Object.keys(groups).map(function (group) {
      return groups[group]; 
    }); 
  }; 
}
