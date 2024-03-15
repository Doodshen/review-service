package service

import (
	"context"
	"fmt"

	pb "review-service/api/review/v1"
	"review-service/internal/biz"
	"review-service/internal/data/model"
)

type ReviewService struct {
	pb.UnimplementedReviewServer

	uc *biz.ReviewUsecase
}

func NewReviewService(uc *biz.ReviewUsecase) *ReviewService {
	return &ReviewService{
		uc: uc,
	}
}

// CreateReview 创建服务
func (s *ReviewService) CreateReview(ctx context.Context, req *pb.CreateReviewRequest) (*pb.CreateReviewReply, error) {
	fmt.Printf("[serviece] CreateReview Req:%#v", req)
	//参数转化 该rpc方法请求体Request 转换为 reviewInfo
	//调用biz层
	var anonymous int32 //判断是不是匿名
	if req.Anonymous {
		anonymous = 1
	}
	review, err := s.uc.CreateReview(ctx, &model.ReviewInfo{
		UserID:       req.UserID,
		OrderID:      req.OrderID,
		Score:        req.Score,
		ExpressScore: req.ExpressScore,
		Content:      req.Content,
		PicInfo:      req.PicInfo,
		VideoInfo:    req.VideoInfo,
		Anonymous:    anonymous,
		Status:       0,
		StoreID:      req.StoreID,
	})

	//如果下一层出现了错误，这里review就是nil，防止空指针
	if err != nil {
		return nil, err

	}
	//拼装返回结果
	return &pb.CreateReviewReply{ReviewID: review.ReviewID}, err

}

// ReplyReview 商家回复评价
func (s *ReviewService) ReplyReview(ctx context.Context, req *pb.ReplyReviewRequest) (*pb.ReplyReviewReply, error) {
	fmt.Printf("[serviece] CreateReview Req:%#v", req)
	//调用biz层
	replyreview, err := s.uc.CreateReply(ctx, &biz.ReplyParam{
		ReviewID:  req.ReviewID,
		StoreID:   req.StoreID,
		Content:   req.Content,
		PicInfo:   req.PicInfo,
		VideoInfo: req.VideoInfo,
	})
	if err != nil {
		return nil, err
	}
	//拼装返回数据

	return &pb.ReplyReviewReply{ReplyID: replyreview.ReplyID}, nil
}

func (s *ReviewService) UpdateReview(ctx context.Context, req *pb.UpdateReviewRequest) (*pb.UpdateReviewReply, error) {
	return &pb.UpdateReviewReply{}, nil
}
func (s *ReviewService) DeleteReview(ctx context.Context, req *pb.DeleteReviewRequest) (*pb.DeleteReviewReply, error) {
	return &pb.DeleteReviewReply{}, nil
}
func (s *ReviewService) GetReview(ctx context.Context, req *pb.GetReviewRequest) (*pb.GetReviewReply, error) {
	return &pb.GetReviewReply{}, nil
}
func (s *ReviewService) ListReview(ctx context.Context, req *pb.ListReviewRequest) (*pb.ListReviewReply, error) {
	return &pb.ListReviewReply{}, nil
}

// AppealReview 申述评价
func (s *ReviewService) AppealReview(ctx context.Context, req *pb.AppealReviewRequest) (*pb.AppealReviewReply, error) {
	fmt.Printf("[service] AppealReview req :%#v\n", req)
	ret, err := s.uc.AppealReview(ctx, &biz.AppealParam{
		ReviewID:  req.GetReviewID(),
		StoreID:   req.GetStoreID(),
		Reason:    req.GetReason(),
		Content:   req.GetContent(),
		PicInfo:   req.GetPicInfo(),
		VideoInfo: req.GetVideoInfo(),
	})
	if err != nil {
		return nil, err
	}
	fmt.Printf("[service AppealReview ret:%v err:%v\n]", ret, err)
	return &pb.AppealReviewReply{AppealID: ret.AppealID}, nil
}

// AuditAppeal O短审核评价
func (s *ReviewService) AuditAppeal(ctx context.Context, req *pb.AuditAppealRequest) (*pb.AuditAppealReply, error) {
	fmt.Printf("[service] AuditAppeal req:%#v\n", req)
	err := s.uc.AuditAppeal(ctx, &biz.AuditAppealParam{
		ReviewID: req.GetReviewID(),
		AppealID: req.GetAppealID(),
		OpUser:   req.GetOpUser(),
		Status:   req.GetStatus(),
	})
	if err != nil {
		return nil, err
	}
	return &pb.AuditAppealReply{}, nil
}

// ListReviewByStoreID 根据商家ID查询评价
func (s *ReviewService) ListReviewByStoreID(ctx context.Context, req *pb.ListReviewByStoreIDRequest) (*pb.ListReviewByStoreIDReply, error) {
	fmt.Printf("[service] ListReviewByStoreID req:%#v\n", req)
	reviewList, err := s.uc.ListReviewByStoreID(ctx, req.StoreID, int(req.Page), int(req.Size))
	if err != nil {
		return nil, err
	}
	//格式化数据，构建Reply
	list := make([]*pb.ReviewInfo, 0, len(reviewList))
	for _, r := range reviewList {
		list = append(list, &pb.ReviewInfo{
			ReviewID:     r.ReviewID,
			UserID:       r.UserID,
			OrderID:      r.OrderID,
			Score:        r.Score,
			ServiceScore: r.ServiceScore,
			ExpressScore: r.ExpressScore,
			Content:      r.Content,
			PicInfo:      r.PicInfo,
			VideoInfo:    r.VideoInfo,
			Status:       r.Status,
		})
	}

	return &pb.ListReviewByStoreIDReply{List: list}, nil
}
