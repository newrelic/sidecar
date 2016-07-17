'use strict';

// Declare app level module which depends on views, and components
angular.module('sidecar', [
  'ngRoute',
  'sidecar.services',
//  'sidecar.version'
]).
config(['$locationProvider', '$routeProvider', function($locationProvider, $routeProvider) {
  $locationProvider.hashPrefix('!');

  $routeProvider.otherwise({redirectTo: '/services'});
}]);
