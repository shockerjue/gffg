package kafka

// The callback interface for subscription messages.
type IConsumer interface {
	// Callback when an error occurs
	Error(error)

	// Callback when Kafka notifies
	// Currently not in use
	Notify(interface{})

	// Callback when subscribing to a message
	// @param	 Partition
	// @param 	 Offset
	// @param	 Value
	Message(int32, int64, []byte)
}
