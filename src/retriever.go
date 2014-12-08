package main

import "github.com/Shopify/sarama"
import "github.com/samuel/go-zookeeper/zk"

import (
	"strconv"
	"time"
)

func connectToZookeeper(zkAdd []string) (zookeeper *zk.Conn, err error) {
	duration, _ := time.ParseDuration(strconv.Itoa(config.zkTimeout) + "ms")
	zookeeper, _, err = zk.Connect(zkAdd, duration)

	return
}

func getKafkaBrokers() (kafkaBrokers []string) {

	return
}

func connectToKafka(kafkaAdd []string) (client *sarama.Client, err error) {
	clientConfig := sarama.NewClientConfig()
	clientConfig.MetadataRetries = 3

	duration, _ := time.ParseDuration("3s")
	clientConfig.WaitForElection = duration

	duration, _ = time.ParseDuration("10s")
	clientConfig.BackgroundRefreshFrequency = duration

	client, err = sarama.NewClient(config.clientId, kafkaAdd, clientConfig)
	return
}

func getAllTopics(client *sarama.Client) []string {
	topics, _ := client.Topics()
	return topics
}

func getAllConsumers(zookeeper *zk.Conn, client *sarama.Client) (liveConsumers []string, deadConsumers []string) {
	consumerNames, _, _ := zookeeper.Children(CONSUMERS)

	for _, consumerId := range consumerNames {
		consumerIdConnections, _, _ := zookeeper.Children(CONSUMERS + "/" + consumerId + IDS)
		if len(consumerIdConnections) != 0 {
			liveConsumers = append(liveConsumers, consumerId)
		} else {
			deadConsumers = append(deadConsumers, consumerId)
		}
	}

	return
}

func getBrokerData(zookeeper *zk.Conn, client *sarama.Client) (brokerData BrokerData, err error) {
	brokerData.Topics = getAllTopics(client)
	alive, dead := getAllConsumers(zookeeper, client)
	brokerData.LiveConsumers = alive
	brokerData.DeadConsumers = dead
	return
}

func getTopicData(client *sarama.Client, topicName string) (topicData []*sarama.TopicMetadata, err error) {
	request := sarama.MetadataRequest{Topics: []string{topicName}}

	partitions, _ := client.Partitions(topicName)
	for _, partition := range partitions {
		broker, _ := client.Leader(topicName, partition)
		response, _ := broker.GetMetadata(config.clientId, &request)
		if response.Topics[0].Err == 0 {
			topicData = response.Topics
		}
	}

	return
}

// cannot use the new OffsetManagement API in Kafka 0.8.1.1
// it's not supported yet - so let's get it from ZooKeeper
func getConsumerData(zookeeper *zk.Conn, client *sarama.Client, consumerId string) (consumerData ConsumerData, err error) {
	consumerData.ConsumerId = consumerId
	consumerData.Live = false
	consumerIdConnections, _, _ := zookeeper.Children(CONSUMERS + "/" + consumerId + IDS)
	if len(consumerIdConnections) != 0 {
		consumerData.Live = true
	}

	topics, _, _ := zookeeper.Children(CONSUMERS + "/" + consumerId + OFFSETS)
	consumerData.Offsets = make([]*ZKConsumerData, 0, len(topics))

	for _, topic := range topics {

		zkData := new(ZKConsumerData)
		zkData.TopicName = topic
		zkData.ConsumerOffset = make(map[string]int64)
		zkData.LatestOffsets = make(map[string]int64)
		zkData.PercentageConsumed = make(map[string]float64)

		partitions, _, _ := zookeeper.Children(CONSUMERS + "/" + consumerId + OFFSETS + "/" + topic)
		for _, partition := range partitions {
			b, _, _ := zookeeper.Get(CONSUMERS + "/" + consumerId + OFFSETS + "/" + topic + "/" + partition)
			offset, _ := strconv.ParseInt(string(b[:]), 10, 0)
			zkData.ConsumerOffset[partition] = offset

			partitionInt, _ := strconv.ParseInt(partition, 10, 0)
			latestOffset, _ := client.GetOffset(topic, int32(partitionInt), sarama.LatestOffsets)
			zkData.LatestOffsets[partition] = latestOffset

			zkData.PercentageConsumed[partition] = (float64(offset) / float64(latestOffset)) * 100
		}
		consumerData.Offsets = append(consumerData.Offsets, zkData)
	}

	return
}
