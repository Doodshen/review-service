package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"review-service/internal/biz"
	"review-service/internal/data/model"
	"review-service/internal/data/query"
	"review-service/pkg/snowflake"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/elastic/go-elasticsearch/v8/typedapi/types"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type reviewRepo struct {
	data *Data
	log  *log.Helper
}

// NewReviewRepo .
func NewReviewRepo(data *Data, logger log.Logger) biz.ReviewRepo {
	return &reviewRepo{
		data: data,
		log:  log.NewHelper(logger),
	}
}

// SaveReview 创建评价
func (r *reviewRepo) SaveReview(ctx context.Context, review *model.ReviewInfo) (*model.ReviewInfo, error) {
	err := r.data.query.ReviewInfo.
		WithContext(ctx).
		Save(review)
	return review, err
}

// GetReviewByOrderID 根据订单ID查询评价
func (r *reviewRepo) GetReviewByOrderID(ctx context.Context, orderID int64) ([]*model.ReviewInfo, error) {
	return r.data.query.ReviewInfo.
		WithContext(ctx).
		Where(r.data.query.ReviewInfo.OrderID.Eq(orderID)).
		Find()
}

// GetReview 查询评价详情
func (r *reviewRepo) GetReview(ctx context.Context, reviewID int64) (*model.ReviewInfo, error) {
	return r.data.query.ReviewInfo.WithContext(ctx).
		Where(r.data.query.ReviewInfo.ReviewID.Eq(reviewID)).
		First()
}

// SaveReply 保存评价回复
func (r *reviewRepo) SaveReply(ctx context.Context, reply *model.ReviewReplyInfo) (*model.ReviewReplyInfo, error) {
	// 1. 数据校验
	// 1.1 数据合法性校验（已回复的评价不允许商家再次回复）
	// 先用评价ID查库,看下是否已回复
	review, err := r.data.query.ReviewInfo.
		WithContext(ctx).
		Where(r.data.query.ReviewInfo.ReviewID.Eq(reply.ReviewID)).
		First()
	if err != nil {
		return nil, err
	}
	if review.HasReply == 1 {
		return nil, errors.New("该评价已回复")
	}
	// 1.2 水平越权校验（A商家只能回复自己的不能回复B商家的）
	// 举例子：用户A删除订单，userID + orderID 当条件去查询订单然后删除
	fmt.Println(review.StoreID, "和", reply.StoreID)
	if review.StoreID != reply.StoreID {
		return nil, errors.New("水平越权")
	}

	// 2. 更新数据库中的数据（评价回复表和评价表要同时更新，涉及到事务操作）
	// 事务操作
	err = r.data.query.Transaction(func(tx *query.Query) error {
		// 回复表插入一条数据
		if err := tx.ReviewReplyInfo.
			WithContext(ctx).
			Save(reply); err != nil {
			r.log.WithContext(ctx).Errorf("SaveReply create reply fail, err:%v", err)
			return err
		}
		// 评价表更新hasReply字段
		if _, err := tx.ReviewInfo.
			WithContext(ctx).
			Where(tx.ReviewInfo.ReviewID.Eq(reply.ReviewID)).
			Update(tx.ReviewInfo.HasReply, 1); err != nil {
			r.log.WithContext(ctx).Errorf("SaveReply update review fail, err:%v", err)
			return err
		}
		return nil
	})
	// 3. 返回
	return reply, err
}

// AppealReview 保存申述内容
func (r *reviewRepo) AppealReview(ctx context.Context, param *biz.AppealParam) (*model.ReviewAppealInfo, error) {
	//1.先查询有没有申述
	ret, err := r.data.query.ReviewAppealInfo.WithContext(ctx).
		Where(
			query.ReviewAppealInfo.ReviewID.Eq(param.ReviewID),
			query.ReviewAppealInfo.StoreID.Eq(param.StoreID),
		).First()
	r.log.Debugf("AppealReview query ret:%v err:%v", ret, err)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		//其他查询错误
		return nil, err
	}

	//已经有了申述评价
	if err == nil && ret.Status > 10 {
		return nil, errors.New("该评价已有审核过的申述记录")
	}

	// 2查询不到审核过的申诉记录
	// 1. 有申诉记录但是处于待审核状态，需要更新
	// if ret != nil{
	// 	// update
	// }else{
	// 	// insert
	// }
	// 3. 没有申诉记录，需要创建

	appeal := &model.ReviewAppealInfo{
		ReviewID:  param.ReviewID,
		StoreID:   param.StoreID,
		Status:    10,
		Reason:    param.Reason,
		Content:   param.Content,
		PicInfo:   param.PicInfo,
		VideoInfo: param.VideoInfo,
	}
	if ret != nil {
		appeal.AppealID = ret.AppealID
	} else {
		appeal.AppealID = snowflake.GenID()
	}
	err = r.data.query.ReviewAppealInfo.
		WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "review_id"}, // ON DUPLICATE KEY
			},
			DoUpdates: clause.Assignments(map[string]interface{}{ // UPDATE
				"status":     appeal.Status,
				"content":    appeal.Content,
				"reason":     appeal.Reason,
				"pic_info":   appeal.PicInfo,
				"video_info": appeal.VideoInfo,
			}),
		}).
		Create(appeal) // INSERT
	r.log.Debugf("AppealReview, err:%v", err)
	return appeal, err

}

// AduitAppeal AuditAppeal 审核申诉（运营对商家的申诉进行审核，审核通过会隐藏该评价）
func (r *reviewRepo) AuditAppeal(ctx context.Context, param *biz.AuditAppealParam) error {
	fmt.Println("这里出现错误2")
	err := r.data.query.Transaction(func(tx *query.Query) error {
		// 申诉表
		if _, err := tx.ReviewAppealInfo.
			WithContext(ctx).
			Where(r.data.query.ReviewAppealInfo.AppealID.Eq(param.AppealID)).
			Updates(map[string]interface{}{
				"status":  param.Status,
				"op_user": param.OpUser,
			}); err != nil {
			return err
		}
		// 评价表
		if param.Status == 20 { // 申诉通过则需要隐藏评价
			if _, err := tx.ReviewInfo.WithContext(ctx).
				Where(tx.ReviewInfo.ReviewID.Eq(param.ReviewID)).
				Update(tx.ReviewInfo.Status, 40); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

// ListReviewByStoreID 根据storeID 分页查询评价
func (r *reviewRepo) ListReviewByStoreID(ctx context.Context, storeID int64, offset, limit int) ([]*biz.MyReviewInfo, error) {
	return r.getData2(ctx, storeID, offset, limit) //直接查询ES
	//return r.getData2(ctx, storeID, offset, limit)
}

// getdata1
func (r *reviewRepo) getdata1(ctx context.Context, storeID int64, offset, limit int) ([]*biz.MyReviewInfo, error) {
	// 去ES里面查询评价
	resp, err := r.data.es.Search().
		Index("review").
		From(offset).
		Size(limit).
		Query(&types.Query{
			Bool: &types.BoolQuery{
				Filter: []types.Query{
					{
						Term: map[string]types.TermQuery{
							"store_id": {Value: storeID},
						},
					},
				},
			},
		}).
		Do(ctx)
	fmt.Printf("--> es search: %v %v\n", resp, err)
	if err != nil {
		return nil, err
	}
	fmt.Printf("es result total:%v\n", resp.Hits.Total.Value)

	//将从es中查询到的数据反序列化-----此处会报错 反序列化时间的时候
	//resp.Hits.Hits[0].Source_--->model.ReviewInfo

	list := make([]*biz.MyReviewInfo, 0, resp.Hits.Total.Value) // //知道了数据的数量，可以直接初始化切片到位
	// list := make([]*model.ReviewInfo)                           // ?

	for _, hit := range resp.Hits.Hits {
		tmp := &biz.MyReviewInfo{}
		if err := json.Unmarshal(hit.Source_, tmp); err != nil { //从es中查询出来的数据是type RawMessage []byte 类型，所以是[]byte类型
			r.log.Errorf("json.Unmarshal(hit.Source_, tmp) failed, err:%v", err)
			continue
		}
		list = append(list, tmp)
	}

	return list, nil
}

var g singleflight.Group

// getData2升级后带有缓存版本的查询函数
func (r *reviewRepo) getData2(ctx context.Context, storeID int64, offset, limit int) ([]*biz.MyReviewInfo, error) {
	//取数据
	//1.先查询redis缓存
	//2 缓存没有查询es
	//3 通过singleflight合并短时间大量的并发请求

	//拼接key
	key := fmt.Sprintf("review:%d:%d:%d", storeID, offset, limit)
	b, err := r.getDataBySingleflight(ctx, key)
	if err != nil {
		return nil, err
	}

	//反序列化
	hm := new(types.HitsMetadata)
	if err := json.Unmarshal(b, hm); err != nil {
		return nil, err
	}
	// 反序列化
	// 反序列化数据
	// resp.Hits.Hits[0].Source_(json.RawMessage)  ==>  model.ReviewInfo
	list := make([]*biz.MyReviewInfo, 0, hm.Total.Value) // ?
	// list := make([]*model.ReviewInfo)                           // ?

	for _, hit := range hm.Hits {
		tmp := &biz.MyReviewInfo{}
		if err := json.Unmarshal(hit.Source_, tmp); err != nil {
			r.log.Errorf("json.Unmarshal(hit.Source_, tmp) failed, err:%v", err)
			continue
		}
		list = append(list, tmp)
	}
	return list, nil
}

// key review:76089:1:10  --> "[{},{},{}]"
// json.Unmarshal([]byte)

func (r *reviewRepo) getDataBySingleflight(ctx context.Context, key string) ([]byte, error) {
	v, err, shared := g.Do(key, func() (interface{}, error) {
		// 查缓存
		data, err := r.getDataFromCache(ctx, key)
		r.log.Debugf("r.getDataFromCache(ctx, key) data:%s, err:%v\n", data, err)
		if err == nil {
			return data, nil
		}
		// 只有在缓存中没有这个key的错误时才查ES
		if errors.Is(err, redis.Nil) {
			// 缓存中没有这个key,说明缓存失效了，需要查ES
			data, err := r.getDataFromES(ctx, key)
			if err == nil {
				// 设置缓存
				return data, r.setCache(ctx, key, data)
			}
			return nil, err
		}
		// 查缓存失败了,直接返回错误，不继续向下传导压力
		return nil, err
	})
	r.log.Debugf("singleflight ret: v:%v err:%v shared:%v\n", v, err, shared)
	if err != nil {
		return nil, err
	}
	return v.([]byte), nil
}

// key review:76089:1:10  --> "[{},{},{}]"
// json.Unmarshal([]byte)   因为es中查询出来的是这个类型，让redis中返回的也是这个类型，这样序列化时，不管是从哪里查询出来的数据都能直接反序列化
// 读取缓存
func (r *reviewRepo) getDataFromCache(ctx context.Context, key string) ([]byte, error) { //这里返回字节类型字符串
	r.log.Debugf("getDataFromCache key:%v\n", key)
	return r.data.rdb.Get(ctx, key).Bytes() //返回bytes类型
}

// setCache设置缓存
func (r *reviewRepo) setCache(ctx context.Context, key string, data []byte) error {
	return r.data.rdb.Set(ctx, key, data, time.Second*10).Err()
}

// getDataFroms 从es中查询
func (r *reviewRepo) getDataFromES(ctx context.Context, key string) ([]byte, error) {
	//分割key从key中获取到page size 等
	values := strings.Split(key, ":")
	if len(values) < 4 {
		return nil, errors.New("invalid key")
	}

	//格式转换
	index, storeID, offsetStr, limitStr := values[0], values[1], values[2], values[3]

	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		return nil, err
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		return nil, err
	}
	resp, err := r.data.es.Search().
		Index(index).
		From(offset).
		Size(limit).
		Query(&types.Query{
			Bool: &types.BoolQuery{
				Filter: []types.Query{
					{
						Term: map[string]types.TermQuery{
							"store_id": {Value: storeID},
						},
					},
				},
			},
		}).
		Do(ctx)
	if err != nil {
		return nil, err
	}

	//将查询到的数据序列化到resp.HitS结构体中
	return json.Marshal(resp.Hits)
}
