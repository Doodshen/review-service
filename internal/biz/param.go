package biz

//ReplyParam 商家回复评价的参数
type ReplyParam struct {
	ReviewID  int64
	StoreID   int64
	Content   string
	PicInfo   string
	VideoInfo string
}

type AppealParam struct {
	ReviewID  int64
	StoreID   int64
	Reason    string
	Content   string
	PicInfo   string
	VideoInfo string
	OpUser    string
}

//AuditAppealParam o端审核商家申述的参数
type AuditParam struct {
	ReviewID int64
	AppealID int64
	OpUser   string
	Status   int32
}
