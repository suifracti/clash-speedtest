package application

import (
	"fmt"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/speedtester"
)

func subscriptionNodeList(body []byte) ([]profiles.SubscriptionNode, error) {
	configs, err := speedtester.SubscriptionNodeConfigs(body)
	if err != nil {
		return nil, err
	}
	nodes := make([]profiles.SubscriptionNode, 0, len(configs))
	for _, cfg := range configs {
		name, _ := cfg["name"].(string)
		nodes = append(nodes, profiles.SubscriptionNode{Identity: monitor.ComputeNodeIdentityKeyFromConfig(cfg), Name: name})
	}
	return nodes, nil
}

func (s *AppService) GetSubscriptionNodeChanges(airportID, subID string) (*profiles.SubscriptionNodeChanges, error) {
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return nil, err
	}
	airport := store.Get(airportID)
	if airport == nil {
		return nil, fmt.Errorf("机场不存在")
	}
	sub := airport.GetSubscription(subID)
	if sub == nil {
		return nil, fmt.Errorf("订阅不存在")
	}
	return sub.NodeVersions.Changes(), nil
}
