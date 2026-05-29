package conf

import (
	"context"
	"fmt"

	"github.com/gofrs/uuid"
	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type QdrantDB struct {
	conn         *grpc.ClientConn
	PointsClient qdrant.PointsClient
	Namespace    uuid.UUID
}

type qdrantAuth struct {
	apiKey string
}

func (a qdrantAuth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"api-key": a.apiKey,
	}, nil
}

func (a qdrantAuth) RequireTransportSecurity() bool {
	// 如果你用了 insecure.NewCredentials()，这里必须返回 false
	return false
}

// NewQdrantClient 初始化客户端 (适配最新 gRPC 规范)
func NewQdrantClient(addr, apiKey string) (*QdrantDB, error) {
	// 使用 NewClient 代替 Dial
	// 注意：NewClient 依然需要传入地址和配置

	creds := credentials.NewClientTLSFromCert(nil, "")

	conn, err := grpc.NewClient(addr,
		// grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(32*1024*1024)),
		grpc.WithPerRPCCredentials(qdrantAuth{apiKey: apiKey}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create qdrant client: %w", err)
	}

	// 全局唯一
	ns, _ := uuid.FromString("144cd804-3d70-446a-8646-d0f11e1a9f43")

	log.Info("Connected to Qdrant! ")
	return &QdrantDB{
		conn:         conn,
		PointsClient: qdrant.NewPointsClient(conn),
		Namespace:    ns,
	}, nil
}

func (q *QdrantDB) Close() {
	q.conn.Close()
}
