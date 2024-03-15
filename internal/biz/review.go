package biz

import (
	"context"
	"fmt"
	v1 "review-service/api/review/v1"
	"review-service/internal/data/model"
	"review-service/pkg/snowflake"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

type ReviewRepo interface {
	SaveReview(context.Context, *model.ReviewInfo) (*model.ReviewInfo, error)
	GetReviewByOrderID(context.Context, int64) ([]*model.ReviewInfo, error)

	SaveReply(context.Context, *model.ReviewReplyInfo) (*model.ReviewReplyInfo, error)

	AppealReview(context.Context, *AppealParam) (*model.ReviewAppealInfo, error)
	AuditAppeal(context.Context, *AuditAppealParam) error

	ListReviewByStoreID(ctx context.Context, storeID int64, offset, limit int) ([]*MyReviewInfo, error)
}

type ReviewUsecase struct {
	repo ReviewRepo
	log  *log.Helper
}

func NewReviewUsecase(repo ReviewRepo, logger log.Logger) *ReviewUsecase {
	return &ReviewUsecase{
		repo: repo,
		log:  log.NewHelper(logger),
	}
}

// CreateReview 创建评价
// 实现业务逻辑的地方
// service层调用该方法
func (uc *ReviewUsecase) CreateReview(ctx context.Context, review *model.ReviewInfo) (*model.ReviewInfo, error) {
	uc.log.WithContext(ctx).Debugf("[biz] CreateReview, req:%v", review)
	// 1、数据校验
	// 1.1 参数基础校验：正常来说不应该放在这一层，你在上一层或者框架层都应该能拦住（validate参数校验）
	// 1.2 参数业务校验：带业务逻辑的参数校验，比如已经评价过的订单不能再创建评价
	reviews, err := uc.repo.GetReviewByOrderID(ctx, review.OrderID)
	if err != nil {
		return nil, v1.ErrorDbFailed("查询数据库失败")
	}
	if len(reviews) > 0 {
		// 已经评价过
		fmt.Printf("订单已评价, len(reviews):%d\n", len(reviews))
		return nil, v1.ErrorOrderReviewed("订单:%d已评价", review.OrderID)
	}
	// 2、生成review ID
	// 这里可以使用雪花算法自己生成
	// 也可以直接接入公司内部的分布式ID生成服务（前提是公司内部有这种服务）
	review.ReviewID = snowflake.GenID()
	// 3、查询订单和商品快照信息
	// 实际业务场景下就需要查询订单服务和商家服务（比如说通过RPC调用订单服务和商家服务）
	// 4、拼装数据入库
	return uc.repo.SaveReview(ctx, review)
}

// CreateReply 创建回复
func (uc *ReviewUsecase) CreateReply(ctx context.Context, param *ReplyParam) (*model.ReviewReplyInfo, error) {
	// 调用data层创建一个评价的回复
	uc.log.WithContext(ctx).Debugf("[biz] CreateReply param:%v", param)
	reply := &model.ReviewReplyInfo{
		ReplyID:   snowflake.GenID(),
		ReviewID:  param.ReviewID,
		StoreID:   param.StoreID,
		Content:   param.Content,
		PicInfo:   param.PicInfo,
		VideoInfo: param.VideoInfo,
	}
	return uc.repo.SaveReply(ctx, reply)
}

// AppealReview 申述评价
func (uc *ReviewUsecase) AppealReview(ctx context.Context, param *AppealParam) (*model.ReviewAppealInfo, error) {
	uc.log.WithContext(ctx).Debugf("[biz] AppealReview param :%v", param)
	return uc.repo.AppealReview(ctx, param)
}

// AduitAppeal 审核申述
func (uc ReviewUsecase) AuditAppeal(ctx context.Context, param *AuditAppealParam) error {
	uc.log.WithContext(ctx).Debugf("[biz] AuditAppeal param:%v", param)
	return uc.repo.AuditAppeal(ctx, param)
}

//ListReviewByStoreID 根据StoreID查询评价

// ListReviewByStoreID 根据storeID分页查询评价
func (uc ReviewUsecase) ListReviewByStoreID(ctx context.Context, storeID int64, page, size int) ([]*MyReviewInfo, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 50 {
		size = 10
	}
	offset := (page - 1) * size
	limit := size
	fmt.Println("这里出错误了")
	uc.log.WithContext(ctx).Debugf("[biz] ListReviewByStoreID storeID:%v", storeID)

	re, err := uc.repo.ListReviewByStoreID(ctx, storeID, offset, limit)
	if err != nil {
		fmt.Println("这里出错了2")
	}
	return re, err
}

//biz层创建MyReviewInfo防止循环引用
//GO语言中时间是这种格式:时间格式化 ：Go语言中的时间是这种格式 ："2006-01-02T15:04:05Z07:00
//解决：自定义时间类型 gen生成的模型中时间的类型不好改 ，可以自定义结构及，将·gen生成的模型进行嵌套然后自定义时间类型

type MyReviewInfo struct {
	*model.ReviewInfo
	CreateAt MyTime `json:"create_time"` //创建时间
	UpdateAt MyTime `json:"update_at"`   //修改时间
}

type MyTime time.Time

func (t *MyTime) UnmarshalJSON(data []byte) error {
	//data = "\"2024-03-15"\"
	s := strings.Trim(string(data), `"`) //去掉引号
	tmp, err := time.Parse(time.DateTime, s)
	if err != nil {
		return err
	}
	*t = MyTime(tmp)
	return nil

}
